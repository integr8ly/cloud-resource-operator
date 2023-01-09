package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	defaultDeploymentDatabase                     = "postgres"
)

type PostgresProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
	TCPPinger         resources.ConnectionTester
}

func NewGCPPostgresProvider(client client.Client, logger *logrus.Entry) *PostgresProvider {
	return &PostgresProvider{
		Client:            client,
		Logger:            logger.WithFields(logrus.Fields{"provider": postgresProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
		TCPPinger:         resources.NewConnectionTestManager(),
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

	cloudSQLCreateConfig, _, strategyConfig, err := p.getPostgresConfig(ctx, pg)
	if err != nil {
		msg := "failed to retrieve postgres strategy config"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, pg.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", pg.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	maintenanceWindowEnabled, err := resources.VerifyPostgresMaintenanceWindow(ctx, p.Client, pg.Namespace, pg.Name)
	if err != nil {
		errMsg := "failed to verify if postgres updates are allowed"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	sqlClient, err := gcpiface.NewSQLAdminService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		errMsg := "could not initialise new SQL Admin Service"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	networkManager, err := NewNetworkManager(ctx, strategyConfig.ProjectID, option.WithCredentialsJSON(creds.ServiceAccountJson), p.Client, logger)
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
	_, err = networkManager.CreateNetworkService(ctx)
	if err != nil {
		msg := "failed to create network service"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	return p.reconcileCloudSQLInstance(ctx, pg, sqlClient, cloudSQLCreateConfig, strategyConfig, maintenanceWindowEnabled, address)
}

func (p *PostgresProvider) reconcileCloudSQLInstance(ctx context.Context, pg *v1alpha1.Postgres, sqladminService gcpiface.SQLAdminService, cloudSQLCreateConfig *gcpiface.DatabaseInstance, strategyConfig *StrategyConfig, maintenanceWindowEnabled bool, address *computepb.Address) (*providers.PostgresInstance, croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "reconcileCloudSQLInstance")
	logger.Infof("reconciling cloudSQL instance")

	sec, err := buildDefaultCloudSQLSecret(pg)
	if err != nil {
		msg := "failed to build default cloudSQL secret"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	result, err := controllerutil.CreateOrUpdate(ctx, p.Client, sec, func() error {
		return nil
	})
	if err != nil {
		errMsg := fmt.Sprintf("failed to create or update secret %s, action was %s", sec.Name, result)
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}


	gcpInstanceConfig, err := p.buildCloudSQLCreateStrategy(ctx, pg, cloudSQLCreateConfig, sec, address)
	if err != nil {
		msg := "failed to build and verify gcp cloudSQL instance configuration"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	foundInstance, err := sqladminService.GetInstance(ctx, strategyConfig.ProjectID, cloudSQLCreateConfig.Name)
	if err != nil && !resources.IsNotFoundError(err) {
		msg := "cannot retrieve sql instance from gcp"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	defer p.exposePostgresInstanceMetrics(ctx, pg, foundInstance)

	if maintenanceWindowEnabled {
		logger.Infof("building cloudSQL update config for: %s", foundInstance.Name)
		modifiedInstance, err := buildCloudSQLUpdateStrategy(cloudSQLCreateConfig, foundInstance, pg)
		if err != nil {
			msg := "error building update config for cloudsql instance"
			return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if modifiedInstance != nil {
			logger.Infof("modifying cloudSQL instance: %s", foundInstance.Name)
			_, err := sqladminService.ModifyInstance(ctx, strategyConfig.ProjectID, foundInstance.Name, modifiedInstance)
			if err != nil && !resources.IsNotStatusConflictError(err) {
				msg := fmt.Sprintf("failed to modify cloudsql instance: %s", foundInstance.Name)
				return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
			}
		}

		_, err = controllerutil.CreateOrUpdate(ctx, p.Client, pg, func() error {
			pg.Spec.MaintenanceWindow = false
			return nil
		})
		if err != nil {
			msg := "failed to set postgres maintenance window to false"
			return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
	}

	if foundInstance == nil {
		logger.Infof("no instance found, creating one")
		_, err := sqladminService.CreateInstance(ctx, strategyConfig.ProjectID, gcpInstanceConfig)
		if err != nil && !resources.IsNotFoundError(err) {
			msg := "failed to create cloudSQL instance"
			return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		annotations.Add(pg, ResourceIdentifierAnnotation, cloudSQLCreateConfig.Name)
		if err := p.Client.Update(ctx, pg); err != nil {
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

	var host string
	for i := range foundInstance.IpAddresses {
		if foundInstance.IpAddresses[i].Type == "PRIVATE" {
			host = foundInstance.IpAddresses[i].IpAddress
		}
	}
	pdd := &providers.PostgresDeploymentDetails{
		Username: string(sec.Data[defaultPostgresUserKey]),
		Password: string(sec.Data[defaultPostgresPasswordKey]),
		Host:     host,
		Database: defaultDeploymentDatabase,
		Port:     defaultGCPPostgresPort,
	}
	return &providers.PostgresInstance{DeploymentDetails: pdd}, "completed cloudSQL instance creation", nil
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

	sqlClient, err := gcpiface.NewSQLAdminService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		errMsg := "could not initialise new SQL Admin Service"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	isLastResource, err := resources.IsLastResource(ctx, p.Client)
	if err != nil {
		errMsg := "failed to check if this cr is the last cr of type postgres and redis"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

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

	return p.deleteCloudSQLInstance(ctx, networkManager, sqlClient, pg, isLastResource)
}

// deleteCloudSQLInstance will retrieve the instance required using the cloudSQLDeleteConfig
// and delete this instance if it is not already pending delete. The credentials and finalizer are then removed.
func (p *PostgresProvider) deleteCloudSQLInstance(ctx context.Context, networkManager NetworkManager, sqladminService gcpiface.SQLAdminService, pg *v1alpha1.Postgres, isLastResource bool) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "deleteCloudSQLInstance")

	_, cloudSQLDeleteConfig, strategyConfig, err := p.getPostgresConfig(ctx, pg)
	if err != nil {
		msg := "failed to retrieve postgres strategy config"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	foundInstance, err := sqladminService.GetInstance(ctx, strategyConfig.ProjectID, cloudSQLDeleteConfig.Name)
	if err != nil && !resources.IsNotFoundError(err) {
		msg := "cannot retrieve sql instance from gcp"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	if foundInstance != nil && foundInstance.Name != "" {
		p.exposePostgresInstanceMetrics(ctx, pg, foundInstance)
		if foundInstance.State == "PENDING_DELETE" {
			statusMessage := fmt.Sprintf("postgres instance %s is already deleting", cloudSQLDeleteConfig.Name)
			p.Logger.Info(statusMessage)
			return croType.StatusMessage(statusMessage), nil
		}
		if !foundInstance.Settings.DeletionProtectionEnabled {
			_, err = sqladminService.DeleteInstance(ctx, strategyConfig.ProjectID, foundInstance.Name)
			if err != nil && !resources.IsNotFoundError(err) {
				msg := fmt.Sprintf("failed to delete postgres instance: %s", cloudSQLDeleteConfig.Name)
				return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
			}
			logrus.Info("triggered Instances.Delete()")
			return "delete detected, Instances.Delete() started", nil
		}

		update := &sqladmin.DatabaseInstance{
			Settings: &sqladmin.Settings{
				ForceSendFields: []string{"DeletionProtectionEnabled"}, DeletionProtectionEnabled: false},
		}

		logrus.Info("modifying instance")
		_, err := sqladminService.ModifyInstance(ctx, strategyConfig.ProjectID, foundInstance.Name, update)
		if err != nil {
			msg := fmt.Sprintf("failed to modify cloudsql instance: %s", cloudSQLDeleteConfig.Name)
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
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
			msg := "failed to delete cluster network service"
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		if err := networkManager.DeleteNetworkIpRange(ctx); err != nil {
			msg := "failed to delete network IP range"
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

// set metrics about the postgres instance being deleted
// works in a similar way to kube_pod_deletion_timestamp
// https://github.com/kubernetes/kube-state-metrics/blob/0bfc2981f9c281c78e33052abdc2d621630562b9/internal/store/pod.go#L200-L218
func (p *PostgresProvider) setPostgresDeletionTimestampMetric(ctx context.Context, pg *v1alpha1.Postgres) {
	if pg.DeletionTimestamp != nil && !pg.DeletionTimestamp.IsZero() {

		instanceName, err := resources.BuildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultGcpIdentifierLength)
		if instanceName == "" {
			logrus.Errorf("unable to build instance name")
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

func (p *PostgresProvider) getPostgresConfig(ctx context.Context, pg *v1alpha1.Postgres) (*gcpiface.DatabaseInstance, *sqladmin.DatabaseInstance, *StrategyConfig, error) {
	strategyConfig, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, pg.Spec.Tier)
	if err != nil {
		errMsg := "failed to read gcp strategy config"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}

	defaultProject, err := GetProjectFromStrategyOrDefault(ctx, p.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp project"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}

	if strategyConfig.ProjectID == "" {
		p.Logger.Debugf("project not set in deployment strategy configuration, using default project %s", defaultProject)
		strategyConfig.ProjectID = defaultProject
	}

	defaultRegion, err := GetRegionFromStrategyOrDefault(ctx, p.Client, strategyConfig)
	if err != nil {
		errMsg := "failed to get default gcp region"
		return nil, nil, nil, errorUtil.Wrap(err, errMsg)
	}
	if strategyConfig.Region == "" {
		p.Logger.Debugf("region not set in deployment strategy configuration, using default region %s", defaultRegion)
		strategyConfig.Region = defaultRegion
	}

	instanceID, err := resources.BuildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultGcpIdentifierLength)
	if err != nil {
		return nil, nil, nil, errorUtil.Wrapf(err, "failed to build cloudsql instance name")
	}

	cloudSQLCreateConfig := &gcpiface.DatabaseInstance{}
	if err := json.Unmarshal(strategyConfig.CreateStrategy, cloudSQLCreateConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal gcp postgres create request")
	}
	if cloudSQLCreateConfig.Name == "" {
		cloudSQLCreateConfig.Name = instanceID
	}

	cloudSQLDeleteConfig := &sqladmin.DatabaseInstance{}
	if err := json.Unmarshal(strategyConfig.DeleteStrategy, cloudSQLDeleteConfig); err != nil {
		return nil, nil, nil, errorUtil.Wrap(err, "failed to unmarshal gcp postgres delete request")
	}
	if cloudSQLDeleteConfig.Name == "" {
		if cloudSQLCreateConfig.Name == "" {
			cloudSQLCreateConfig.Name = instanceID
		}
		cloudSQLDeleteConfig.Name = cloudSQLCreateConfig.Name
	}

	return cloudSQLCreateConfig, cloudSQLDeleteConfig, strategyConfig, nil
}


func (p *PostgresProvider) buildCloudSQLCreateStrategy(ctx context.Context, pg *v1alpha1.Postgres, cloudSQLCreateConfig *gcpiface.DatabaseInstance, sec *v1.Secret, address *computepb.Address) (*sqladmin.DatabaseInstance, error) {

	if cloudSQLCreateConfig.DatabaseVersion == "" {
		cloudSQLCreateConfig.DatabaseVersion = defaultGCPCLoudSQLDatabaseVersion
	}

	if cloudSQLCreateConfig.Region == "" {
		cloudSQLCreateConfig.Region = defaultGCPCloudSQLRegion
	}

	instanceName, err := resources.BuildInfraNameFromObject(ctx, p.Client, pg.ObjectMeta, defaultGcpIdentifierLength)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to build instance name")
	}
	if cloudSQLCreateConfig.Name == "" {
		cloudSQLCreateConfig.Name = instanceName
	}

	if cloudSQLCreateConfig.RootPassword == "" {
		cloudSQLCreateConfig.RootPassword = string(sec.Data[defaultPostgresPasswordKey])
	}

	tags, err := buildDefaultPostgresTags(ctx, p.Client, pg)
	if err != nil {
		return nil, nil, errorUtil.Wrap(err, "failed to build gcp postgres instance tags")
	}

	if cloudSQLCreateConfig.Settings == nil {
		cloudSQLCreateConfig.Settings = &gcpiface.Settings{
			Tier:                   defaultTier,
			AvailabilityType:       defaultAvailabilityType,
			StorageAutoResizeLimit: defaultStorageAutoResizeLimit,
			StorageAutoResize:      utils.Bool(defaultStorageAutoResize),
			BackupConfiguration: &gcpiface.BackupConfiguration{
				Enabled:                    utils.Bool(defaultBackupConfigEnabled),
				PointInTimeRecoveryEnabled: utils.Bool(defaultPointInTimeRecoveryEnabled),
				BackupRetentionSettings: &gcpiface.BackupRetentionSettings{
					RetentionUnit:   defaultBackupRetentionSettingsRetentionUnit,
					RetainedBackups: defaultBackupRetentionSettingsRetainedBackups,
				},
			},
			DataDiskSizeGb:            defaultDataDiskSizeGb,
			DeletionProtectionEnabled: utils.Bool(defaultDeleteProtectionEnabled),
			IpConfiguration: &gcpiface.IpConfiguration{
				AllocatedIpRange: address.GetName(),
				Ipv4Enabled: utils.Bool(defaultIPConfigIPV4Enabled),
				PrivateNetwork:   strings.Split(address.GetNetwork(), "v1/")[1],
			},
			UserLabels: tags,
		}
	}
	if cloudSQLCreateConfig.Settings.UserLabels == nil {
		cloudSQLCreateConfig.Settings.UserLabels = map[string]string{}
	}
	for key, value := range tags {
		cloudSQLCreateConfig.Settings.UserLabels[key] = value
	}

	if cloudSQLCreateConfig.Settings.Tier == "" {
		cloudSQLCreateConfig.Settings.Tier = defaultTier
	}
	if cloudSQLCreateConfig.Settings.AvailabilityType == "" {
		cloudSQLCreateConfig.Settings.AvailabilityType = defaultAvailabilityType
	}
	if cloudSQLCreateConfig.Settings.StorageAutoResizeLimit == 0 {
		cloudSQLCreateConfig.Settings.StorageAutoResizeLimit = defaultStorageAutoResizeLimit
	}
	if cloudSQLCreateConfig.Settings.BackupConfiguration == nil {
		cloudSQLCreateConfig.Settings.BackupConfiguration = &gcpiface.BackupConfiguration{
			Enabled:                    utils.Bool(defaultBackupConfigEnabled),
			PointInTimeRecoveryEnabled: utils.Bool(defaultPointInTimeRecoveryEnabled),
			BackupRetentionSettings: &gcpiface.BackupRetentionSettings{
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

	gcpInstanceConfig, err := convertDatabaseStruct(cloudSQLCreateConfig)
	if err != nil {
		errMsg := "failed to convert database struct"
		return nil, errorUtil.Wrap(err, errMsg)
	}

	return gcpInstanceConfig, nil
}

func buildCloudSQLUpdateStrategy(cloudSQLConfig *gcpiface.DatabaseInstance, foundInstanceConfig *sqladmin.DatabaseInstance, pg *v1alpha1.Postgres) (*sqladmin.DatabaseInstance, error) {
	logrus.Infof("verifying that %s configuration is as expected", foundInstanceConfig.Name)
	updateFound := false
	modifiedInstance := &sqladmin.DatabaseInstance{}

	if cloudSQLConfig.Region != foundInstanceConfig.Region {
		modifiedInstance.Region = cloudSQLConfig.Region
		updateFound = true
	}

	if cloudSQLConfig.Settings != nil && foundInstanceConfig.Settings != nil {
		modifiedInstance.Settings = &sqladmin.Settings{
			ForceSendFields: []string{},
		}

		if cloudSQLConfig.Settings.DeletionProtectionEnabled != nil && *cloudSQLConfig.Settings.DeletionProtectionEnabled != foundInstanceConfig.Settings.DeletionProtectionEnabled {
			modifiedInstance.Settings.DeletionProtectionEnabled = *cloudSQLConfig.Settings.DeletionProtectionEnabled
			modifiedInstance.Settings.ForceSendFields = append(modifiedInstance.Settings.ForceSendFields, "DeletionProtectionEnabled")
			updateFound = true
		}

		if cloudSQLConfig.Settings.StorageAutoResize != nil && *cloudSQLConfig.Settings.StorageAutoResize != *foundInstanceConfig.Settings.StorageAutoResize {
			modifiedInstance.Settings.StorageAutoResize = cloudSQLConfig.Settings.StorageAutoResize
			modifiedInstance.Settings.ForceSendFields = append(modifiedInstance.Settings.ForceSendFields, "StorageAutoResize")
			updateFound = true
		}

		if cloudSQLConfig.Settings.Tier != foundInstanceConfig.Settings.Tier {
			modifiedInstance.Settings.Tier = cloudSQLConfig.Settings.Tier
			updateFound = true
		}
		if cloudSQLConfig.Settings.AvailabilityType != foundInstanceConfig.Settings.AvailabilityType {
			modifiedInstance.Settings.AvailabilityType = cloudSQLConfig.Settings.AvailabilityType
			updateFound = true
		}
		if cloudSQLConfig.Settings.StorageAutoResizeLimit != foundInstanceConfig.Settings.StorageAutoResizeLimit {
			modifiedInstance.Settings.StorageAutoResizeLimit = cloudSQLConfig.Settings.StorageAutoResizeLimit
			updateFound = true
		}
		if cloudSQLConfig.Settings.DataDiskSizeGb != foundInstanceConfig.Settings.DataDiskSizeGb {
			modifiedInstance.Settings.DataDiskSizeGb = cloudSQLConfig.Settings.DataDiskSizeGb
			updateFound = true
		}
	}

	if cloudSQLConfig.Settings.BackupConfiguration != nil && foundInstanceConfig.Settings.BackupConfiguration != nil {
		modifiedInstance.Settings.BackupConfiguration = &sqladmin.BackupConfiguration{
			ForceSendFields: []string{},
		}
		if cloudSQLConfig.Settings.BackupConfiguration.Enabled != nil && *cloudSQLConfig.Settings.BackupConfiguration.Enabled != foundInstanceConfig.Settings.BackupConfiguration.Enabled {
			modifiedInstance.Settings.BackupConfiguration.Enabled = *cloudSQLConfig.Settings.BackupConfiguration.Enabled
			modifiedInstance.Settings.BackupConfiguration.ForceSendFields = append(modifiedInstance.Settings.BackupConfiguration.ForceSendFields, "Enabled")
			updateFound = true
		}
		if cloudSQLConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled != nil && *cloudSQLConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled != foundInstanceConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled {
			modifiedInstance.Settings.BackupConfiguration.PointInTimeRecoveryEnabled = *cloudSQLConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled
			modifiedInstance.Settings.BackupConfiguration.ForceSendFields = append(modifiedInstance.Settings.BackupConfiguration.ForceSendFields, "PointInTimeRecoveryEnabled")
			updateFound = true
		}
	}

	if cloudSQLConfig.Settings.BackupConfiguration.BackupRetentionSettings != nil && foundInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings != nil {
		modifiedInstance.Settings.BackupConfiguration.BackupRetentionSettings = &sqladmin.BackupRetentionSettings{}

		if cloudSQLConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit != foundInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit {
			modifiedInstance.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit = cloudSQLConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit
			updateFound = true
		}
		if cloudSQLConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups != foundInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups {
			modifiedInstance.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups = cloudSQLConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups
			updateFound = true
		}
	}

	if cloudSQLConfig.Settings.IpConfiguration != nil && foundInstanceConfig.Settings.IpConfiguration != nil {
		modifiedInstance.Settings.IpConfiguration = &sqladmin.IpConfiguration{
			ForceSendFields: []string{},
		}
		if cloudSQLConfig.Settings.IpConfiguration != nil && foundInstanceConfig.Settings.IpConfiguration != nil {
			if cloudSQLConfig.Settings.IpConfiguration.Ipv4Enabled != nil && *cloudSQLConfig.Settings.IpConfiguration.Ipv4Enabled != foundInstanceConfig.Settings.IpConfiguration.Ipv4Enabled {
				modifiedInstance.Settings.IpConfiguration.Ipv4Enabled = *cloudSQLConfig.Settings.IpConfiguration.Ipv4Enabled
				modifiedInstance.Settings.IpConfiguration.ForceSendFields = append(modifiedInstance.Settings.IpConfiguration.ForceSendFields, "Ipv4Enabled")
				updateFound = true
			}
		}
	}

	if cloudSQLConfig.DatabaseVersion != "" {
		newVersion, existingVersion := formatGcpPostgresVersion(cloudSQLConfig.DatabaseVersion, foundInstanceConfig.DatabaseVersion)
		versionUpgradeNeeded, err := resources.VerifyVersionUpgradeNeeded(existingVersion, newVersion)
		if err != nil {
			return nil, errorUtil.Wrap(err, "failed to parse database version")
		}
		if versionUpgradeNeeded {
			logrus.Info(fmt.Sprintf("Version upgrade found, the current DatabaseVersion is %s and is upgrading to %s", foundInstanceConfig.DatabaseVersion, cloudSQLConfig.DatabaseVersion))
			modifiedInstance.DatabaseVersion = cloudSQLConfig.DatabaseVersion
			updateFound = true
		}
	}

	if !updateFound {
		return nil, nil
	}

	return modifiedInstance, nil
}

func (p *PostgresProvider) exposePostgresInstanceMetrics(ctx context.Context, pg *v1alpha1.Postgres, instance *sqladmin.DatabaseInstance) {
	if instance == nil {
		return
	}
	clusterID, err := resources.GetClusterID(ctx, p.Client)
	if err != nil {
		p.Logger.Errorf("failed to get cluster id while exposing metrics for postgres instance %s", instance.Name)
		return
	}
	genericLabels := resources.BuildGenericMetricLabels(pg.ObjectMeta, clusterID, instance.Name, postgresProviderName)
	instanceState := instance.State
	infoLabels := resources.BuildInfoMetricLabels(pg.ObjectMeta, instanceState, clusterID, instance.Name, postgresProviderName)
	resources.SetMetricCurrentTime(resources.DefaultPostgresInfoMetricName, infoLabels)
	// a single metric should be exposed for each possible status phase
	// the value of the metric should be 1.0 when the resource is in that phase
	// the value of the metric should be 0.0 when the resource is not in that phase
	for _, phase := range []croType.StatusPhase{croType.PhaseFailed, croType.PhaseDeleteInProgress, croType.PhasePaused, croType.PhaseComplete, croType.PhaseInProgress} {
		labelsFailed := resources.BuildStatusMetricsLabels(pg.ObjectMeta, clusterID, instance.Name, postgresProviderName, phase)
		resources.SetMetric(resources.DefaultPostgresStatusMetricName, labelsFailed, resources.Btof64(pg.Status.Phase == phase))
	}
	// set availability metric, based on the status flag on the cloudsql postgres instance in gcp
	// the value of the metric should be 0 when the instance state is unhealthy
	// the value of the metric should be 1 when the instance state is healthy
	// more details on possible state values here: https://pkg.go.dev/google.golang.org/api/sqladmin/v1beta4@v0.105.0#DatabaseInstance.State
	var instanceHealthy float64
	var instanceConnectable float64
	if resources.Contains(healthyPostgresInstanceStates(), instanceState) {
		instanceHealthy = 1
		if len(instance.IpAddresses) > 0 {
			var host string
			for i := range instance.IpAddresses {
				if instance.IpAddresses[i].Type == "PRIVATE" {
					host = instance.IpAddresses[i].IpAddress
				}
			}
			if success := p.TCPPinger.TCPConnection(host, defaultGCPPostgresPort); success {
				instanceConnectable = 1
			}
		}
	}
	resources.SetMetric(resources.DefaultPostgresAvailMetricName, genericLabels, instanceHealthy)
	resources.SetMetric(resources.DefaultPostgresConnectionMetricName, genericLabels, instanceConnectable)
}

func healthyPostgresInstanceStates() []string {
	return []string{
		"PENDING_CREATE",
		"RUNNABLE",
		"PENDING_DELETE",
	}
}

func buildDefaultCloudSQLSecret(p *v1alpha1.Postgres) (*v1.Secret, error) {
	password, err := resources.GeneratePassword()
	if err != nil {
		errMsg := "failed to generate password"
		return nil, errorUtil.Wrap(err, errMsg)
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
	}, nil
}

func convertDatabaseStruct(cloudSQLCreateConfig *gcpiface.DatabaseInstance) (*sqladmin.DatabaseInstance, error) {
	gcpInstanceConfig := &sqladmin.DatabaseInstance{}

	if cloudSQLCreateConfig.ConnectionName != "" {
		gcpInstanceConfig.ConnectionName = cloudSQLCreateConfig.ConnectionName
	}
	if cloudSQLCreateConfig.DatabaseVersion != "" {
		gcpInstanceConfig.DatabaseVersion = cloudSQLCreateConfig.DatabaseVersion
	}
	if cloudSQLCreateConfig.DiskEncryptionConfiguration != nil {
		gcpInstanceConfig.DiskEncryptionConfiguration = cloudSQLCreateConfig.DiskEncryptionConfiguration
	}
	if cloudSQLCreateConfig.FailoverReplica != nil {
		gcpInstanceConfig.FailoverReplica = &sqladmin.DatabaseInstanceFailoverReplica{}
		if cloudSQLCreateConfig.FailoverReplica.Available != nil {
			gcpInstanceConfig.FailoverReplica.Available = *cloudSQLCreateConfig.FailoverReplica.Available
		}
		if cloudSQLCreateConfig.FailoverReplica.Name != "" {
			gcpInstanceConfig.FailoverReplica.Name = cloudSQLCreateConfig.FailoverReplica.Name
		}
	}
	if cloudSQLCreateConfig.GceZone != "" {
		gcpInstanceConfig.GceZone = cloudSQLCreateConfig.GceZone
	}
	if cloudSQLCreateConfig.InstanceType != "" {
		gcpInstanceConfig.InstanceType = cloudSQLCreateConfig.InstanceType
	}
	if cloudSQLCreateConfig.IpAddresses != nil {
		gcpInstanceConfig.IpAddresses = cloudSQLCreateConfig.IpAddresses
	}
	if cloudSQLCreateConfig.Kind != "" {
		gcpInstanceConfig.Kind = cloudSQLCreateConfig.Kind
	}
	if cloudSQLCreateConfig.MaintenanceVersion != "" {
		gcpInstanceConfig.MaintenanceVersion = cloudSQLCreateConfig.MaintenanceVersion
	}
	if cloudSQLCreateConfig.MasterInstanceName != "" {
		gcpInstanceConfig.MasterInstanceName = cloudSQLCreateConfig.MasterInstanceName
	}
	if cloudSQLCreateConfig.MaxDiskSize != 0 {
		gcpInstanceConfig.MaxDiskSize = cloudSQLCreateConfig.MaxDiskSize
	}
	if cloudSQLCreateConfig.Name != "" {
		gcpInstanceConfig.Name = cloudSQLCreateConfig.Name
	}
	if cloudSQLCreateConfig.Project != "" {
		gcpInstanceConfig.Project = cloudSQLCreateConfig.Project
	}
	if cloudSQLCreateConfig.Region != "" {
		gcpInstanceConfig.Region = cloudSQLCreateConfig.Region
	}
	if cloudSQLCreateConfig.ReplicaNames != nil {
		gcpInstanceConfig.ReplicaNames = cloudSQLCreateConfig.ReplicaNames
	}
	if cloudSQLCreateConfig.RootPassword != "" {
		gcpInstanceConfig.RootPassword = cloudSQLCreateConfig.RootPassword
	}
	if cloudSQLCreateConfig.SecondaryGceZone != "" {
		gcpInstanceConfig.GceZone = cloudSQLCreateConfig.SecondaryGceZone
	}
	if cloudSQLCreateConfig.SelfLink != "" {
		gcpInstanceConfig.SelfLink = cloudSQLCreateConfig.SelfLink
	}
	if cloudSQLCreateConfig.ServerCaCert != nil {
		gcpInstanceConfig.ServerCaCert = cloudSQLCreateConfig.ServerCaCert
	}
	if cloudSQLCreateConfig.Settings != nil {
		gcpInstanceConfig.Settings = &sqladmin.Settings{}
		if cloudSQLCreateConfig.Settings.ActivationPolicy != "" {
			gcpInstanceConfig.Settings.ActivationPolicy = cloudSQLCreateConfig.Settings.ActivationPolicy
		}
		if cloudSQLCreateConfig.Settings.AvailabilityType != "" {
			gcpInstanceConfig.Settings.AvailabilityType = cloudSQLCreateConfig.Settings.AvailabilityType
		}
		if cloudSQLCreateConfig.Settings.BackupConfiguration != nil {
			gcpInstanceConfig.Settings.BackupConfiguration = &sqladmin.BackupConfiguration{}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings != nil {
				gcpInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings = &sqladmin.BackupRetentionSettings{}
				if cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit != "" {
					gcpInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit = cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetentionUnit
				}
				if cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups != 0 {
					gcpInstanceConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups = cloudSQLCreateConfig.Settings.BackupConfiguration.BackupRetentionSettings.RetainedBackups
				}
			}

			if cloudSQLCreateConfig.Settings.BackupConfiguration.BinaryLogEnabled != nil {
				gcpInstanceConfig.Settings.BackupConfiguration.BinaryLogEnabled = *cloudSQLCreateConfig.Settings.BackupConfiguration.BinaryLogEnabled
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.Enabled != nil {
				gcpInstanceConfig.Settings.BackupConfiguration.Enabled = *cloudSQLCreateConfig.Settings.BackupConfiguration.Enabled
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.Kind != "" {
				gcpInstanceConfig.Settings.BackupConfiguration.Kind = cloudSQLCreateConfig.Settings.BackupConfiguration.Kind
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.Location != "" {
				gcpInstanceConfig.Settings.BackupConfiguration.Location = cloudSQLCreateConfig.Settings.BackupConfiguration.Location
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled != nil {
				gcpInstanceConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled = *cloudSQLCreateConfig.Settings.BackupConfiguration.PointInTimeRecoveryEnabled
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.ReplicationLogArchivingEnabled != nil {
				gcpInstanceConfig.Settings.BackupConfiguration.ReplicationLogArchivingEnabled = *cloudSQLCreateConfig.Settings.BackupConfiguration.ReplicationLogArchivingEnabled
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.StartTime != "" {
				gcpInstanceConfig.Settings.BackupConfiguration.StartTime = cloudSQLCreateConfig.Settings.BackupConfiguration.StartTime
			}
			if cloudSQLCreateConfig.Settings.BackupConfiguration.TransactionLogRetentionDays != 0 {
				gcpInstanceConfig.Settings.BackupConfiguration.TransactionLogRetentionDays = cloudSQLCreateConfig.Settings.BackupConfiguration.TransactionLogRetentionDays
			}
		}
		if cloudSQLCreateConfig.Settings.Collation != "" {
			gcpInstanceConfig.Settings.Collation = cloudSQLCreateConfig.Settings.Collation
		}
		if cloudSQLCreateConfig.Settings.ConnectorEnforcement != "" {
			gcpInstanceConfig.Settings.ConnectorEnforcement = cloudSQLCreateConfig.Settings.ConnectorEnforcement
		}
		if cloudSQLCreateConfig.Settings.CrashSafeReplicationEnabled != nil {
			gcpInstanceConfig.Settings.CrashSafeReplicationEnabled = *cloudSQLCreateConfig.Settings.CrashSafeReplicationEnabled
		}
		if cloudSQLCreateConfig.Settings.DataDiskSizeGb != 0 {
			gcpInstanceConfig.Settings.DataDiskSizeGb = cloudSQLCreateConfig.Settings.DataDiskSizeGb
		}
		if cloudSQLCreateConfig.Settings.DataDiskType != "" {
			gcpInstanceConfig.Settings.DataDiskType = cloudSQLCreateConfig.Settings.DataDiskType
		}
		if cloudSQLCreateConfig.Settings.DatabaseFlags != nil {
			gcpInstanceConfig.Settings.DatabaseFlags = cloudSQLCreateConfig.Settings.DatabaseFlags
		}
		if cloudSQLCreateConfig.Settings.DatabaseReplicationEnabled != nil {
			gcpInstanceConfig.Settings.DatabaseReplicationEnabled = *cloudSQLCreateConfig.Settings.DatabaseReplicationEnabled
		}
		if cloudSQLCreateConfig.Settings.DeletionProtectionEnabled != nil {
			gcpInstanceConfig.Settings.DeletionProtectionEnabled = *cloudSQLCreateConfig.Settings.DeletionProtectionEnabled
		}
		if cloudSQLCreateConfig.Settings.DenyMaintenancePeriods != nil {
			gcpInstanceConfig.Settings.DenyMaintenancePeriods = cloudSQLCreateConfig.Settings.DenyMaintenancePeriods
		}
		if cloudSQLCreateConfig.Settings.InsightsConfig != nil {
			gcpInstanceConfig.Settings.InsightsConfig = cloudSQLCreateConfig.Settings.InsightsConfig
		}
		if cloudSQLCreateConfig.Settings.IpConfiguration != nil {
			gcpInstanceConfig.Settings.IpConfiguration = &sqladmin.IpConfiguration{}
			if cloudSQLCreateConfig.Settings.IpConfiguration.AllocatedIpRange != "" {
				gcpInstanceConfig.Settings.IpConfiguration.AllocatedIpRange = cloudSQLCreateConfig.Settings.IpConfiguration.AllocatedIpRange
			}
			if cloudSQLCreateConfig.Settings.IpConfiguration.AuthorizedNetworks != nil {
				gcpInstanceConfig.Settings.IpConfiguration.AuthorizedNetworks = cloudSQLCreateConfig.Settings.IpConfiguration.AuthorizedNetworks
			}
			if cloudSQLCreateConfig.Settings.IpConfiguration.AuthorizedNetworks != nil {
				gcpInstanceConfig.Settings.IpConfiguration.AuthorizedNetworks = cloudSQLCreateConfig.Settings.IpConfiguration.AuthorizedNetworks
			}
			if cloudSQLCreateConfig.Settings.IpConfiguration.Ipv4Enabled != nil {
				gcpInstanceConfig.Settings.IpConfiguration.Ipv4Enabled = *cloudSQLCreateConfig.Settings.IpConfiguration.Ipv4Enabled
			}
			if cloudSQLCreateConfig.Settings.IpConfiguration.PrivateNetwork != "" {
				gcpInstanceConfig.Settings.IpConfiguration.PrivateNetwork = cloudSQLCreateConfig.Settings.IpConfiguration.PrivateNetwork
			}
			if cloudSQLCreateConfig.Settings.IpConfiguration.RequireSsl != nil {
				gcpInstanceConfig.Settings.IpConfiguration.RequireSsl = *cloudSQLCreateConfig.Settings.IpConfiguration.RequireSsl
			}
		}
		if cloudSQLCreateConfig.Settings.Kind != "" {
			gcpInstanceConfig.Settings.Kind = cloudSQLCreateConfig.Settings.Kind
		}
		if cloudSQLCreateConfig.Settings.LocationPreference != nil {
			gcpInstanceConfig.Settings.LocationPreference = cloudSQLCreateConfig.Settings.LocationPreference
		}
		if cloudSQLCreateConfig.Settings.MaintenanceWindow != nil {
			gcpInstanceConfig.Settings.MaintenanceWindow = cloudSQLCreateConfig.Settings.MaintenanceWindow
		}
		if cloudSQLCreateConfig.Settings.PasswordValidationPolicy != nil {
			gcpInstanceConfig.Settings.PasswordValidationPolicy = cloudSQLCreateConfig.Settings.PasswordValidationPolicy
		}
		if cloudSQLCreateConfig.Settings.PricingPlan != "" {
			gcpInstanceConfig.Settings.PricingPlan = cloudSQLCreateConfig.Settings.PricingPlan
		}
		if cloudSQLCreateConfig.Settings.ReplicationType != "" {
			gcpInstanceConfig.Settings.ReplicationType = cloudSQLCreateConfig.Settings.ReplicationType
		}
		if cloudSQLCreateConfig.Settings.SettingsVersion != 0 {
			gcpInstanceConfig.Settings.SettingsVersion = cloudSQLCreateConfig.Settings.SettingsVersion
		}
		if cloudSQLCreateConfig.Settings.StorageAutoResize != nil {
			gcpInstanceConfig.Settings.StorageAutoResize = cloudSQLCreateConfig.Settings.StorageAutoResize
		}
		if cloudSQLCreateConfig.Settings.StorageAutoResizeLimit != 0 {
			gcpInstanceConfig.Settings.StorageAutoResizeLimit = cloudSQLCreateConfig.Settings.StorageAutoResizeLimit
		}
		if cloudSQLCreateConfig.Settings.Tier != "" {
			gcpInstanceConfig.Settings.Tier = cloudSQLCreateConfig.Settings.Tier
		}
		if cloudSQLCreateConfig.Settings.UserLabels != nil {
			gcpInstanceConfig.Settings.UserLabels = cloudSQLCreateConfig.Settings.UserLabels
		}
	}
	return gcpInstanceConfig, nil
}

func formatGcpPostgresVersion(gcpNewVersion string, gcpExistingVersion string) (semverNewVersion string, semverExistingVersion string) {
	cloudSQLConfigSemverDatabaseVersion := strings.Replace(gcpNewVersion, "_", ".", -1)
	semverNewVersion = strings.TrimPrefix(cloudSQLConfigSemverDatabaseVersion, "POSTGRES.")
	foundInstanceConfigSemverDatabaseVersion := strings.Replace(gcpExistingVersion, "_", ".", -1)
	semverExistingVersion = strings.TrimPrefix(foundInstanceConfigSemverDatabaseVersion, "POSTGRES.")
	return semverNewVersion, semverExistingVersion
}
