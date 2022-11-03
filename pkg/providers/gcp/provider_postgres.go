package gcp

import (
	"context"
	"fmt"
	"time"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	postgresProviderName         = "gcp-cloudsql"
	ResourceIdentifierAnnotation = "resourceIdentifier"
	defaultCredSecSuffix         = "-gcp-sql-credentials"
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

	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, pg.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", pg.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	// TODO: replace with strategy config
	projectID, err := resources.GetGCPProject(ctx, p.Client)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	networkManager, err := NewNetworkManager(ctx, projectID, option.WithCredentialsJSON(creds.ServiceAccountJson), p.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	// get cidr block from _network strat map, based on tier from postgres cr
	ipRangeCidr, err := networkManager.ReconcileNetworkProviderConfig(ctx, p.ConfigManager, pg.Spec.Tier, logger)
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

	// TODO implement me
	return p.createCloudSQLInstance(ctx, pg)
}

func (p *PostgresProvider) createCloudSQLInstance(ctx context.Context, pg *v1alpha1.Postgres) (*providers.PostgresInstance, croType.StatusMessage, error) {
	return nil, croType.StatusEmpty, nil
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
func (p *PostgresProvider) deleteCloudSQLInstance(ctx context.Context, projectID string, networkManager NetworkManager, sqladminService gcpiface.SQLAdminService, pg *v1alpha1.Postgres, isLastResource bool) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "deleteCloudSQLInstance")

	instances, err := getCloudSQLInstances(sqladminService, projectID)
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

	var foundInstance *sqladmin.DatabaseInstance
	for _, instance := range instances {
		if instance.Name == instanceName {
			logrus.Infof("found matching instance by name: %s", instanceName)
			foundInstance = instance
			break
		}
	}

	if foundInstance != nil {
		if foundInstance.State == "PENDING_DELETE" {
			statusMessage := fmt.Sprintf("postgres instance %s is already deleting", instanceName)
			p.Logger.Info(statusMessage)
			return croType.StatusMessage(statusMessage), nil
		}

		_, err = sqladminService.DeleteInstance(ctx, projectID, instanceName)
		if err != nil {
			msg := fmt.Sprintf("failed to delete postgres instance: %s", instanceName)
			return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
		}
		logrus.Info("triggered Instances.Delete()")
		return "delete detected, Instances.Delete() started", nil
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
