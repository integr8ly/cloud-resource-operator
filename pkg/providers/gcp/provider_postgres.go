package gcp

import (
	"context"
	"fmt"
	"time"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ providers.PostgresProvider = (*PostgresProvider)(nil)

const (
	postgresProviderName         = "gcp"
	projectID                    = "rhoam-317914"
	ResourceIdentifierAnnotation = "resourceIdentifier"
	defaultCredSecSuffix         = "-gcp-sql-credentials"
)

type PostgresProvider struct {
	Client            client.Client
	Logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
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
	_, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return nil, croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	if err := resources.CreateFinalizer(ctx, pp.Client, p, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	// TODO implement me
	return nil, "", nil
}

func (pp *PostgresProvider) DeletePostgres(ctx context.Context, p *v1alpha1.Postgres) (croType.StatusMessage, error) {
	logger := pp.Logger.WithField("action", "DeletePostgres")
	logger.Infof("reconciling postgres %s", p.Name)

	// set postgres deletion timestamp metric
	pp.setPostgresDeletionTimestampMetric(ctx, p)

	// get provider gcp creds to access the postgres instance
	creds, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return croType.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}

	// build cloudSQL service
	sqladminService, err := sqladmin.NewService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson))
	if err != nil {
		return "error building cloudSQL admin service", err
	}

	// get cloudSQL instance
	instances, err := getCloudSQLInstances(sqladminService)
	if err != nil {
		return "cannot retrieve sql instances from gcp", err
	}
	logrus.Info("listed sql instances from gcp")

	instanceName := annotations.Get(p, ResourceIdentifierAnnotation)

	if instanceName == "" {
		msg := "unable to find instance name from annotation"
		return croType.StatusMessage(msg), fmt.Errorf(msg)
	}

	// check if instance exists
	var foundInstance *sqladmin.DatabaseInstance
	for _, instance := range instances {
		if instance.Name == instanceName {
			logrus.Infof("found matching instance bu name: %s", instanceName)
			foundInstance = instance
			break
		}
	}
	// check instance state
	if foundInstance != nil {
		if foundInstance.State != "RUNNABLE" {
			statusMessage := fmt.Sprintf("delete detected, DeletePostgres() is in progress, current cloudSQL status is %s", foundInstance.State)
			logger.Info(statusMessage)
			return croType.StatusMessage(statusMessage), nil
		}
		// delete if not in progress
		_, err = sqladminService.Instances.Delete(projectID, instanceName).Context(ctx).Do()
		if err != nil {
			return croType.StatusMessage(fmt.Sprintf("failed to delete cloudSQL instance: %s", instanceName)), err
		}
		logrus.Info("triggered Instances.Delete()")
		return "delete detected, Instances.Delete() started", nil
	}

	// delete credential secret
	logger.Info("deleting cloudSQL secret")
	sec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name + defaultCredSecSuffix,
			Namespace: p.Namespace,
		},
	}
	err = pp.Client.Delete(ctx, sec)
	if err != nil && !k8serr.IsNotFound(err) {
		msg := "failed to deleted cloudSQL secrets"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	// remove finalizer
	resources.RemoveFinalizer(&p.ObjectMeta, DefaultFinalizer)
	if err := pp.Client.Update(ctx, p); err != nil {
		msg := "failed to update instance as part of finalizer reconcile"
		return croType.StatusMessage(msg), errorUtil.Wrapf(err, msg)
	}

	return croType.StatusEmpty, nil
}

func getCloudSQLInstances(sqladminService *sqladmin.Service) ([]*sqladmin.DatabaseInstance, error) {
	// check for existing instance
	instances, err := sqladminService.Instances.List(projectID).Do()
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
		// build instance name

		instanceName := annotations.Get(p, ResourceIdentifierAnnotation)

		if instanceName == "" {
			logrus.Errorf("unable to find instance name from annotation")
		}
		// get Cluster Id
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
