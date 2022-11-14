package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/annotations"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	utils "k8s.io/utils/pointer"
)

const (
	postgresProviderName                          = "gcp-cloudsql"
	ResourceIdentifierAnnotation                  = "resourceIdentifier"
	defaultCredSecSuffix                          = "-gcp-sql-credentials"
	defaultGCPCLoudSQLDatabaseVersion             = "POSTGRES_13"
	defaultGCPCloudSQLRegion                      = "us-central1"
	defaultGCPIdentifierLength                    = 40
	defaultGCPPostgresUser                        = "postgres"
	defaultPostgresPasswordKey                    = "password"
	defaultPostgresUserKey                        = "user"
	defaultTier                                   = "db-custom-2-3840"
	defaultAvailabilityType                       = "REGIONAL"
	defaultStorageAutoResizeLimit                 = 100
	defaultStorageAutoResize                      = true
	defaultBackupConfigEnabled                    = true
	defaultPointInTimeRecoveryEnabled             = true
	defaultBackupRetentionSettingsRetentionUnit   = "COUNT"
	defaultBackupRetentionSettingsRetainedBackups = 30
	defaultDataDiskSizeGb                         = 20
	defaultDeleteProtectionEnabled                = true
	defaultIPConfigIPV4Enabled                    = true
	defaultGCPPostgresPort                        = 5432
)

type PostgresProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

// wrapper for real client
type sqlClient struct {
	gcpiface.SQLAdminService
	sqlAdminService *sqladmin.Service
}

func (r *sqlClient) InstancesList(project string) (*sqladmin.InstancesListResponse, error) {
	return r.sqlAdminService.Instances.List(project).Do()
}

func (r *sqlClient) DeleteInstance(ctx context.Context, projectID, instanceName string) (*sqladmin.Operation, error) {
	return r.sqlAdminService.Instances.Delete(projectID, instanceName).Context(ctx).Do()
}

func (r *sqlClient) CreateInstance(ctx context.Context, projectID string, databaseInstance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
	return r.sqlAdminService.Instances.Insert(projectID, databaseInstance).Context(ctx).Do()
}

func (r *sqlClient) ModifyInstance(ctx context.Context, projectID string, instanceName string, databaseInstance *sqladmin.DatabaseInstance) (*sqladmin.Operation, error) {
	logrus.Info("coming to you from MODIFY INSTANCE FUNCTION")
	return r.sqlAdminService.Instances.Patch(projectID, instanceName, databaseInstance).Context(ctx).Do()
}

func NewGCPPostgresProvider(client client.Client, logger *logrus.Entry) *PostgresProvider {
	return &PostgresProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": postgresProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (p *PostgresProvider) GetName() string {
	return postgresProviderName
}

func (p *PostgresProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (p *PostgresProvider) GetReconcileTime(pg *v1alpha1.Postgres) time.Duration {
	if pg.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (p *PostgresProvider) ReconcilePostgres(ctx context.Context, pg *v1alpha1.Postgres) (*providers.PostgresInstance, croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "CreatePostgres")
	if err := resources.CreateFinalizer(ctx, p.Client, pg, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	createInstancesRequest, _, strategyConfig, err := pp.getPostgresConfig(ctx, p)
	if err != nil {
		msg := "failed to retrieve postgres strategy config"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	creds, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", pg.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	maintenanceWindow, err := resources.VerifyPostgresMaintenanceWindow(ctx, pp.Client, p.Namespace, p.Name)
	if err != nil {
		msg := "failed to verify if postgres updates are allowed"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	sec := buildDefaultCloudSQLSecret(p)
	result, err := controllerutil.CreateOrUpdate(ctx, pp.Client, sec, func() error {
		return nil
	})
	if err != nil {
		errMsg := fmt.Sprintf("failed to create or update secret %s, action was %s", sec.Name, result)
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	sqladminService, err := sqladmin.NewService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		errMsg := "error building cloudSQL admin service"
		return nil, croType.StatusMessage(errMsg), err
	}
	sqlClient := &sqlClient{
		sqlAdminService: sqladminService,
	}

	networkManager, err := NewNetworkManager(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), pp.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	// get cidr block from _network strat map, based on tier from postgres cr
	ipRangeCidr, err := networkManager.ReconcileNetworkProviderConfig(ctx, p.ConfigManager, pg.Spec.Tier)
	if err != nil {
		errMsg := "failed to reconcile network provider config"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	address, err := networkManager.CreateNetworkIpRange(ctx, ipRangeCidr)
	if err != nil {
		msg := "failed to create network service"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	if address == nil || address.GetStatus() == computepb.Address_RESERVING.String() {
		return nil, croType.StatusMessage("network ip address range creation in progress"), nil
	}
	logger.Infof("created ip address range %s: %s/%d", address.GetName(), address.GetAddress(), address.GetPrefixLength())

	logger.Infof("creating network service connection")
	service, err := networkManager.CreateNetworkService(ctx)
	if err != nil {
		msg := "failed to create network service"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	if service == nil {
		return nil, croType.StatusMessage("network service connection creation in progress"), nil
	}
	logger.Infof("created network service connection %s", service.Service)

	return pp.reconcileCloudSQLInstance(ctx, p, sqlClient, createInstancesRequest, strategyConfig, maintenanceWindow)
}

func (pp *PostgresProvider) reconcileCloudSQLInstance(ctx context.Context, p *v1alpha1.Postgres, sqladminService gcpiface.SQLAdminService, cloudSQLCreateConfig *sqladmin.DatabaseInstance, strategyConfig *StrategyConfig, maintenanceWindow bool) (*providers.PostgresInstance, croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "reconcileCloudSQLInstance")
	logger.Infof("reconciling cloudSQL instance")

	instances, err := getCloudSQLInstances(sqladminService, strategyConfig.ProjectID)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	logrus.Info("listed sql instances from gcp")

	credSec := &v1.Secret{}
	if err := pp.Client.Get(ctx, types.NamespacedName{Name: p.Name + defaultCredSecSuffix, Namespace: p.Namespace}, credSec); err != nil {
		msg := "failed to retrieve cloudSQL credential secret"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	postgresPass := string(credSec.Data[defaultPostgresPasswordKey])
	if postgresPass == "" {
		msg := "unable to retrieve postgres password"
		return nil, croType.StatusMessage(msg), errorUtil.Errorf(msg)
	}

	if err := pp.buildCloudSQLCreateStrategy(ctx, p, cloudSQLCreateConfig); err != nil {
		msg := "failed to build and verify gcp cloudSQL instance configuration"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	foundInstance := getFoundInstance(instances, cloudSQLCreateConfig.Name)

	// TODO setPostgresServiceMaintenanceMetric
	// TODO exposePostgresMetrics
	// TODO createCloudSQLConnectionMetric

	defer pp.createCloudSQLConnectionMetric(ctx, p, foundInstance)

	// TODO if foundInstance != nil -> update strategy MGDAPI-4900

	if foundInstance == nil {
		logger.Infof("no instance found, creating one")
		_, err := sqladminService.CreateInstance(ctx, strategyConfig.ProjectID, cloudSQLCreateConfig)
		if err != nil {
			msg := "failed to create gcp instance"
			return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		annotations.Add(p, ResourceIdentifierAnnotation, cloudSQLCreateConfig.Name)
		if err := pp.Client.Update(ctx, p); err != nil {
			msg := "failed to add annotation"
			return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		msg := "started cloudSQL provision"
		return nil, croType.StatusMessage(msg), nil
	}
	if foundInstance.State == "PENDING_CREATE" {
		msg := fmt.Sprintf("creation of %s cloudSQL instance in progress", foundInstance.Name)
		logger.Infof(msg)
		return nil, croType.StatusMessage(msg), nil
	}

	return nil, "started cloudSQL provision", nil
}

// DeletePostgres will set the postgres deletion timestamp, reconcile provider credentials so that the postgres instance
// can be accessed, build the cloudSQL service using these credentials and call the deleteCloudSQLInstance function to
// perform the delete action.
func (p *PostgresProvider) DeletePostgres(ctx context.Context, pg *v1alpha1.Postgres) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "DeletePostgres")
	logger.Infof("reconciling postgres %s", pg.Name)

	p.setPostgresDeletionTimestampMetric(ctx, pg)

	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, pg.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", pg.Name)
		return croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	sqladminService, err := sqladmin.NewService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		return "error building cloudSQL admin service", err
	}
	sqlClient := &sqlClient{
		sqlAdminService: sqladminService,
	}

	isLastResource, err := resources.IsLastResource(ctx, p.Client)
	if err != nil {
		errMsg := "failed to check if this cr is the last cr of type postgres and redis"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// TODO: replace with strategy config
	projectID, err := resources.GetGCPProject(ctx, p.Client)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	networkManager, err := NewNetworkManager(ctx, projectID, option.WithCredentialsJSON(creds.ServiceAccountJson), p.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	return p.deleteCloudSQLInstance(ctx, projectID, networkManager, sqlClient, pg, isLastResource)
}

// deleteCloudSQLInstance will retrieve instances from gcp, find the instance required using the resourceIdentifierAnnotation
// and delete this instance if it is not already pending delete. The credentials and finalizer are then removed.
func (pp *PostgresProvider) deleteCloudSQLInstance(ctx context.Context, networkManager NetworkManager, sqladminService gcpiface.SQLAdminService, p *v1alpha1.Postgres, isLastResource bool) (croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "deleteCloudSQLInstance")

	_, deleteInstanceRequest, strategyConfig, err := pp.getPostgresConfig(ctx, p)
	if err != nil {
		msg := "failed to retrieve postgres strategy config"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	instances, err := getCloudSQLInstances(sqladminService, strategyConfig.ProjectID)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	logrus.Info("listed sql instances from gcp")

	instanceName := annotations.Get(pg, ResourceIdentifierAnnotation)

	if instanceName == "" {
		msg := "unable to find instance name from annotation"
		return croType.StatusMessage(msg), fmt.Errorf(msg)
	}

	foundInstance := getFoundInstance(instances, instanceName)

	if foundInstance != nil {
		if foundInstance.State == "PENDING_DELETE" {
			statusMessage := fmt.Sprintf("postgres instance %s is already deleting", instanceName)
			p.Logger.Info(statusMessage)
			return croType.StatusMessage(statusMessage), nil
		}
		if foundInstance.Name == "" {
			statusMessage := "unable to get name from instance delete call"
			return croType.StatusMessage(statusMessage), nil
		}
		if !foundInstance.Settings.DeletionProtectionEnabled {
			_, err = sqladminService.DeleteInstance(ctx, strategyConfig.ProjectID, deleteInstanceRequest.Name)
			if err != nil {
				msg := fmt.Sprintf("failed to delete postgres instance: %s", instanceName)
				return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
			}
			logrus.Info("triggered Instances.Delete()")
			return "delete detected, Instances.Delete() started", nil
		}

		update := &sqladmin.DatabaseInstance{
			Settings: &sqladmin.Settings{DeletionProtectionEnabled: false},
		}

		logrus.Info("modifying instance")
		resp, err := sqladminService.ModifyInstance(ctx, strategyConfig.ProjectID, foundInstance.Name, update)
		if err != nil {
			msg := fmt.Sprintf("failed to modify cloudsql instance: %s", instanceName)
			logrus.Infof("modify instance response: %v", resp)
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		logrus.Infof("MODIFIED instance DPA %v", foundInstance.Settings.DeletionProtectionEnabled)
		return croType.StatusMessage("modifying instance"), nil
	}

	logger.Info("deleting cloudSQL secret")
	sec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pg.Name + defaultCredSecSuffix,
			Namespace: pg.Namespace,
		},
	}
	err = p.Client.Delete(ctx, sec)
	if err != nil && !k8serr.IsNotFound(err) {
		msg := "failed to delete cloudSQL secrets"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// remove networking components
	if isLastResource {
		if err := networkManager.DeleteNetworkPeering(ctx); err != nil {
			msg := "failed to delete cluster network peering"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if err := networkManager.DeleteNetworkService(ctx); err != nil {
			msg := "failed to delete cluster network peering"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if err := networkManager.DeleteNetworkIpRange(ctx); err != nil {
			msg := "failed to delete aws networking"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if exist, err := networkManager.ComponentsExist(ctx); err != nil || exist {
			if exist {
				return croType.StatusMessage("network component deletion in progress"), nil
			}
			msg := "failed to check if components exist"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
	}

	resources.RemoveFinalizer(&pg.ObjectMeta, DefaultFinalizer)
	if err := p.Client.Update(ctx, pg); err != nil {
		msg := "failed to update instance as part of finalizer reconcile"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	return croType.StatusEmpty, nil
}

func getCloudSQLInstances(service gcpiface.SQLAdminService, projectID string) ([]*sqladmin.DatabaseInstance, error) {
	instances, err := service.InstancesList(projectID)
	if err != nil {
		return nil, err
	}

	return instances.Items, nil
}

// set metrics about the postgres instance being deleted
// works in a similar way to kube_pod_deletion_timestamp
// https://github.com/kubernetes/kube-state-metrics/blob/0bfc2981f9c281c78e33052abdc2d621630562b9/internal/store/pod.go#L200-L218
func (p *PostgresProvider) setPostgresDeletionTimestampMetric(ctx context.Context, pg *v1alpha1.Postgres) {
	if pg.DeletionTimestamp != nil && !pg.DeletionTimestamp.IsZero() {

		instanceName := annotations.Get(pg, ResourceIdentifierAnnotation)

		if instanceName == "" {
			logrus.Errorf("unable to find instance name from annotation")
		}

		logrus.Info("setting postgres information metric")
		clusterID, err := resources.GetClusterID(ctx, p.Client)
		if err != nil {
			logrus.Errorf("failed to get cluster id while exposing information metric for %v", instanceName)
			return
		}

		labels := buildPostgresStatusMetricsLabels(pg, clusterID, instanceName, pg.Status.Phase)
		resources.SetMetric(resources.DefaultPostgresDeletionMetricName, labels, float64(pg.DeletionTimestamp.Unix()))
	}
}

func buildPostgresGenericMetricLabels(pg *v1alpha1.Postgres, clusterID, instanceName string) map[string]string {
	labels := map[string]string{}
	labels["clusterID"] = clusterID
	labels["resourceID"] = pg.Name
	labels["namespace"] = pg.Namespace
	labels["instanceID"] = instanceName
	labels["productName"] = pg.Labels["productName"]
	labels["strategy"] = postgresProviderName
	return labels
}

func buildPostgresStatusMetricsLabels(cr *v1alpha1.Postgres, clusterID, instanceName string, phase croType.StatusPhase) map[string]string {
	labels := buildPostgresGenericMetricLabels(cr, clusterID, instanceName)
	labels["statusPhase"] = string(phase)
	return labels
}

func (pp *PostgresProvider) getPostgresConfig(ctx context.Context, p *v1alpha1.Postgres) (*sqladmin.DatabaseInstance, *sqladmin.DatabaseInstance, *StrategyConfig, error) {
	strategyConfig, err := pp.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, p.Spec.Tier)
	logrus.Infof("GET POSTGRES CONFIG strategyConfig.CreateStrategy %v", strategyConfig.CreateStrategy)
	if err != nil {
		errMsg := "failed to read gcp strategy config"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}

	defaultProject, err := GetProjectFromStrategyOrDefault(ctx, pp.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp project"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}

	if strategyConfig.ProjectID == "" {
		pp.Logger.Debugf("project not set in deployment strategy configuration, using default project %s", defaultProject)
		strategyConfig.ProjectID = defaultProject
	}

	defaultRegion, err := GetRegionFromStrategyOrDefault(ctx, pp.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp region"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}
	if strategyConfig.Region == "" {
		pp.Logger.Debugf("region not set in deployment strategy configuration, using default region %s", defaultRegion)
		strategyConfig.Region = defaultRegion
	}

	createInstanceRequest, deleteInstanceRequest, err := pp.buildPostgresConfig(p, strategyConfig)
	if err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to build postgres config")
	}
	return createInstanceRequest, deleteInstanceRequest, strategyConfig, nil
}

func (pp *PostgresProvider) buildPostgresConfig(p *v1alpha1.Postgres, strategyConfig *StrategyConfig) (*sqladmin.DatabaseInstance, *sqladmin.DatabaseInstance, error) {
	createInstanceRequest := &sqladmin.DatabaseInstance{}
	logrus.Infof("buildpostgresconfig createinstancerequest %v", createInstanceRequest)
	if err := json.Unmarshal(strategyConfig.CreateStrategy, createInstanceRequest); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal gcp postgres create request")
	}
	if createInstanceRequest.Name == "" {
		instanceName := annotations.Get(p, ResourceIdentifierAnnotation)
		if instanceName == "" {
			errMsg := "failed to find postgres instance name from annotation"
			return nil, nil, fmt.Errorf(errMsg)
		}
	}

	deleteInstanceRequest := &sqladmin.DatabaseInstance{}
	if err := json.Unmarshal(strategyConfig.DeleteStrategy, deleteInstanceRequest); err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to unmarshal gcp postgres delete request")
	}
	if deleteInstanceRequest.Name == "" {
		instanceName := annotations.Get(p, ResourceIdentifierAnnotation)
		if instanceName == "" {
			errMsg := "failed to find postgres instance name from annotation"
			return nil, nil, fmt.Errorf(errMsg)
		}
	}
	return createInstanceRequest, deleteInstanceRequest, nil
}

func (pp *PostgresProvider) buildCloudSQLCreateStrategy(ctx context.Context, p *v1alpha1.Postgres, cloudSQLCreateConfig *sqladmin.DatabaseInstance) error {
	logrus.Infof("buildCloudSQLCreateStratefy %v", cloudSQLCreateConfig)

	if cloudSQLCreateConfig.DatabaseVersion == "" {
		cloudSQLCreateConfig.DatabaseVersion = defaultGCPCLoudSQLDatabaseVersion
	}

	if cloudSQLCreateConfig.Region == "" {
		cloudSQLCreateConfig.Region = defaultGCPCloudSQLRegion
	}

	instanceName, err := pp.buildInstanceName(ctx, p)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to build instance name")
	}
	if cloudSQLCreateConfig.Name == "" {
		cloudSQLCreateConfig.Name = instanceName
	}

	rootPassword := buildDefaultCloudSQLSecret(p)
	if cloudSQLCreateConfig.RootPassword == "" {
		cloudSQLCreateConfig.RootPassword = rootPassword.String()
	}

	if cloudSQLCreateConfig.Settings == nil {
		cloudSQLCreateConfig.Settings = &sqladmin.Settings{
			Tier:                   defaultTier,
			AvailabilityType:       defaultAvailabilityType,
			StorageAutoResizeLimit: defaultStorageAutoResizeLimit,
			StorageAutoResize:      utils.Bool(defaultStorageAutoResize),
			BackupConfiguration: &sqladmin.BackupConfiguration{
				Enabled:                    defaultBackupConfigEnabled,
				PointInTimeRecoveryEnabled: defaultPointInTimeRecoveryEnabled,
				BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
					RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
					RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
				},
			},
			DataDiskSizeGb:            defaultDataDiskSizeGb,
			DeletionProtectionEnabled: defaultDeleteProtectionEnabled,
			IpConfiguration: &sqladmin.IpConfiguration{
				Ipv4Enabled: defaultIPConfigIPV4Enabled,
			},
		}
	}
	if cloudSQLCreateConfig.Settings.Tier == "" {
		cloudSQLCreateConfig.Settings.Tier = defaultTier
	}
	if cloudSQLCreateConfig.Settings.AvailabilityType == "" {
		cloudSQLCreateConfig.Settings.Tier = defaultAvailabilityType
	}
	if cloudSQLCreateConfig.Settings.StorageAutoResizeLimit == 0 {
		cloudSQLCreateConfig.Settings.StorageAutoResizeLimit = defaultStorageAutoResizeLimit
	}
	if cloudSQLCreateConfig.Settings.BackupConfiguration == nil {
		cloudSQLCreateConfig.Settings.BackupConfiguration = &sqladmin.BackupConfiguration{
			Enabled:                    defaultBackupConfigEnabled,
			PointInTimeRecoveryEnabled: defaultPointInTimeRecoveryEnabled,
			BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
				RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
				RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
			},
		}
	}
	if cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit == "" {
		cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit = defaultBackupRetentionSettingsRetentionUnit
	}
	if cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups == 0 {
		cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups = defaultBackupRetentionSettingsRetainedBackups
	}
	if cloudSQLCreateConfig.Settings.DataDiskSizeGb == 0 {
		cloudSQLCreateConfig.Settings.DataDiskSizeGb = defaultDataDiskSizeGb
	}
	return nil
}

func (pp *PostgresProvider) buildInstanceName(ctx context.Context, p *v1alpha1.Postgres) (string, error) {
	instanceName, err := resources.BuildInfraNameFromObject(ctx, pp.Client, p.ObjectMeta, defaultGCPIdentifierLength)
	if err != nil {
		return "", errorUtil.Errorf("error occured building instance name: %v", err)
	}
	return instanceName, nil
}

func buildDefaultCloudSQLSecret(p *v1alpha1.Postgres) *v1.Secret {
	password, err := resources.GeneratePassword()
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name + defaultCredSecSuffix,
			Namespace: p.Namespace,
		},
		StringData: map[string]string{
			defaultPostgresUserKey:     defaultGCPPostgresUser,
			defaultPostgresPasswordKey: password,
		},
		Type: v1.SecretTypeOpaque,
	}
}

func (pp *PostgresProvider) createCloudSQLConnectionMetric(ctx context.Context, p *v1alpha1.Postgres, instance *sqladmin.DatabaseInstance) {
	instanceName, err := pp.buildInstanceName(ctx, p)
	if err != nil {
		logrus.Errorf("error occurred while building instance name during postgres metrics: %v", err)
	}
	logrus.Infof("testing and exposing postgres connection metric for: %s", instanceName)
	clusterID, err := resources.GetClusterID(ctx, pp.Client)
	if err != nil {
		logrus.Errorf("failed to get cluster id while exposing connection metric for %v", instanceName)
	}
	genericLabels := buildPostgresGenericMetricLabels(p, clusterID, instanceName)

	if instance == nil {
		logrus.Infof("foundInstance is nil, setting createCloudSQLConnectionMetric to 0")
		resources.SetMetric(resources.DefaultPostgresConnectionMetricName, genericLabels, 0)
		return
	}
	logrus.Infof("CONTENTS of instance : %v", instance)
	if instance.IpAddresses == nil {
		logrus.Infof("instance endpoint not yet available for: %s", instance.Name)
		resources.SetMetric(resources.DefaultPostgresConnectionMetricName, genericLabels, 0)
		return
	}

}

func getFoundInstance(instances []*sqladmin.DatabaseInstance, instanceName string) *sqladmin.DatabaseInstance {
	var foundInstance *sqladmin.DatabaseInstance
	for _, i := range instances {
		if i.Name == instanceName {
			foundInstance = i
			return foundInstance
		}
	}
	return nil
}
