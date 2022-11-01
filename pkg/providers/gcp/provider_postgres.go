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

func (pp *PostgresProvider) GetName() string {
	return postgresProviderName
}

func (pp *PostgresProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (pp *PostgresProvider) GetReconcileTime(p *v1alpha1.Postgres) time.Duration {
	if p.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (pp *PostgresProvider) ReconcilePostgres(ctx context.Context, p *v1alpha1.Postgres) (*providers.PostgresInstance, croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "CreatePostgres")
	if err := resources.CreateFinalizer(ctx, pp.Client, p, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	creds, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	networkManager, err := NewNetworkManager(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), pp.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	address, err := networkManager.CreateNetworkIpRange(ctx)
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
	return pp.createCloudSQLInstance(ctx, p)
}

func (pp *PostgresProvider) createCloudSQLInstance(ctx context.Context, p *v1alpha1.Postgres) (*providers.PostgresInstance, croType.StatusMessage, error) {
	return nil, croType.StatusEmpty, nil
}

// DeletePostgres will set the postgres deletion timestamp, reconcile provider credentials so that the postgres instance
// can be accessed, build the cloudSQL service using these credentials and call the deleteCloudSQLInstance function to
// perform the delete action.
func (pp *PostgresProvider) DeletePostgres(ctx context.Context, p *v1alpha1.Postgres) (croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "DeletePostgres")
	logger.Infof("reconciling postgres %s", p.Name)

	pp.setPostgresDeletionTimestampMetric(ctx, p)

	creds, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	sqladminService, err := sqladmin.NewService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		return "error building cloudSQL admin service", err
	}
	sqlClient := &sqlClient{
		sqlAdminService: sqladminService,
	}

	isLastResource, err := resources.IsLastResource(ctx, pp.Client)
	if err != nil {
		errMsg := "failed to check if this cr is the last cr of type postgres and redis"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	networkManager, err := NewNetworkManager(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), pp.Client, logger)
	if err != nil {
		errMsg := "failed to initialise network manager"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	return pp.deleteCloudSQLInstance(ctx, networkManager, sqlClient, p, isLastResource)
}

// deleteCloudSQLInstance will retrieve instances from gcp, find the instance required using the resourceIdentifierAnnotation
// and delete this instance if it is not already pending delete. The credentials and finalizer are then removed.
func (pp *PostgresProvider) deleteCloudSQLInstance(ctx context.Context, networkManager NetworkManager, sqladminService gcpiface.SQLAdminService, p *v1alpha1.Postgres, isLastResource bool) (croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "deleteCloudSQLInstance")

	// TODO: replace with strategy config
	projectID, err := resources.GetGCPProject(ctx, pp.Client)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	instances, err := getCloudSQLInstances(sqladminService, projectID)
	if err != nil {
		msg := "cannot retrieve sql instances from gcp"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	logrus.Info("listed sql instances from gcp")

	instanceName := annotations.Get(p, ResourceIdentifierAnnotation)

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
			pp.Logger.Info(statusMessage)
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
			Name:      p.Name + defaultCredSecSuffix,
			Namespace: p.Namespace,
		},
	}
	err = pp.Client.Delete(ctx, sec)
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

	resources.RemoveFinalizer(&p.ObjectMeta, DefaultFinalizer)
	if err := pp.Client.Update(ctx, p); err != nil {
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
func (pp *PostgresProvider) setPostgresDeletionTimestampMetric(ctx context.Context, p *v1alpha1.Postgres) {
	if p.DeletionTimestamp != nil && !p.DeletionTimestamp.IsZero() {

		instanceName := annotations.Get(p, ResourceIdentifierAnnotation)

		if instanceName == "" {
			logrus.Errorf("unable to find instance name from annotation")
		}

		logrus.Info("setting postgres information metric")
		clusterID, err := resources.GetClusterID(ctx, pp.Client)
		if err != nil {
			logrus.Errorf("failed to get cluster id while exposing information metric for %v", instanceName)
			return
		}

		labels := buildPostgresStatusMetricsLabels(p, clusterID, instanceName, p.Status.Phase)
		resources.SetMetric(resources.DefaultPostgresDeletionMetricName, labels, float64(p.DeletionTimestamp.Unix()))
	}
}

func buildPostgresGenericMetricLabels(p *v1alpha1.Postgres, clusterID, instanceName string) map[string]string {
	labels := map[string]string{}
	labels["clusterID"] = clusterID
	labels["resourceID"] = p.Name
	labels["namespace"] = p.Namespace
	labels["instanceID"] = instanceName
	labels["productName"] = p.Labels["productName"]
	labels["strategy"] = postgresProviderName
	return labels
}

func buildPostgresStatusMetricsLabels(cr *v1alpha1.Postgres, clusterID, instanceName string, phase croType.StatusPhase) map[string]string {
	labels := buildPostgresGenericMetricLabels(cr, clusterID, instanceName)
	labels["statusPhase"] = string(phase)
	return labels
}
