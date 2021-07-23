package gcp

import (
	"context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/annotations"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

var _ providers.PostgresProvider = (*PostgresProvider)(nil)

const (
	postgresProviderName       = "gcp-cloudsql"
	projectID                  = "rhoam-317914"
	defaultCredSecSuffix       = "-gcp-sql-credentials"
	defaultGCPPostgresUser     = "postgres"
	defaultPostgresUserKey     = "postgres"
	defaultPostgresPasswordKey = "password"
	defaultGCPDBInstanceTier   = "db-f1-micro"
	defaultGCPDatabaseVersion  = "POSTGRES_13"
	defaultGCPPostgresPort     = 5432
)

type PostgresProvider struct {
	Client client.Client
	Logger *logrus.Entry
}

func NewGCPPostgresProvider(client client.Client, logger *logrus.Entry) *PostgresProvider {
	return &PostgresProvider{
		Client: client,
		Logger: logger.WithFields(logrus.Fields{"provider": postgresProviderName}),
	}
}

func (p PostgresProvider) GetName() string {
	return postgresProviderName
}

func (p PostgresProvider) SupportsStrategy(s string) bool {
	return s == providers.GCPDeploymentStrategy
}

func (p PostgresProvider) GetReconcileTime(ps *v1alpha1.Postgres) time.Duration {
	if ps.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

// CreatePostgres creates a postgres instances with the credentials at the
// path specified in the GOOGLE_APPLICATION_CREDENTIALS env
func (p PostgresProvider) ReconcilePostgres(ctx context.Context, ps *v1alpha1.Postgres) (*providers.PostgresInstance, croType.StatusMessage, error) {

	// handle provider-specific finalizer
	if err := resources.CreateFinalizer(ctx, p.Client, ps, DefaultFinalizer); err != nil {
		return nil, "failed to set finalizer", err
	}

	// Build default cloudSQL secret
	secret := buildDefaultCloudSQLSecret(ps)
	or, err := controllerutil.CreateOrUpdate(ctx, p.Client, secret, func() error {
		return nil
	})
	if err != nil {
		errMsg := fmt.Sprintf("failed to create or update secret %s, action was %s", secret.Name, or)
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}

	// Build cloudSQL service
	sqladminService, err := sqladmin.NewService(ctx)
	if err != nil {
		return nil, "error building cloud sql admin service", err
	}

	// create cloudSQL instance
	return p.createCloudSQLInstance(ctx, ps, sqladminService)
}

func (p *PostgresProvider) createCloudSQLInstance(ctx context.Context, cr *v1alpha1.Postgres,
	sqladminService *sqladmin.Service) (*providers.PostgresInstance, croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "createCloudSQLInstance")

	// getting postgres user password from created secret
	credSec := &v1.Secret{}
	if err := p.Client.Get(ctx, types.NamespacedName{Name: cr.Name + defaultCredSecSuffix, Namespace: cr.Namespace}, credSec); err != nil {
		msg := "failed to retrieve rds credential secret"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	postgresPass := string(credSec.Data[defaultPostgresPasswordKey])
	if postgresPass == "" {
		msg := "unable to retrieve rds password"
		return nil, croType.StatusMessage(msg), fmt.Errorf(msg)
	}

	// Create cloudSQL config struct
	cloudSQLConfig, err := p.getCloudSQLConfig(ctx, cr, postgresPass)
	if err != nil {
		return nil, "Could not get cloudSQL config", err
	}

	// TODO: populate cloudSQL config with default values if not set

	instances, err := getCloudSQLInstances(sqladminService)
	if err != nil {
		return nil, "Cannot retrieve sql instances from gcp", err
	}
	logrus.Info("listed sql instances from gcp")

	// check if instance already exist
	var foundInstance *sqladmin.DatabaseInstance
	for _, instance := range instances {
		if instance.Name == cloudSQLConfig.Name {
			logrus.Infof("found matching intance by name: %s", cloudSQLConfig.Name)
			foundInstance = instance
			break
		}
	}

	// Create if not found
	if foundInstance == nil {
		logger.Infof("no instance found, creating one")
		_, err := sqladminService.Instances.Insert(projectID, cloudSQLConfig).Do()
		if err != nil {
			return nil, croType.StatusMessage(fmt.Sprintf("Failed to create gcp instace: %v", cloudSQLConfig)), err
		}

		annotations.Add(cr, ResourceIdentifierAnnotation, cloudSQLConfig.Name)
		if err := p.Client.Update(ctx, cr); err != nil {
			return nil, "failed to add annotation", err
		}

		return nil, "started cloud sql provision", nil
	}

	if foundInstance.State != "RUNNABLE" {
		msg := fmt.Sprintf("createCloudSQLInstance() in progress, current gcp sql resource status is %s", foundInstance.State)
		logger.Infof(msg)
		return nil, croType.StatusMessage(msg), nil
	}

	postgresInstance, err := fromDatabaseInstance(foundInstance, postgresPass)
	if err != nil {
		return nil, "Could not create postgres instance from database instance", err
	}

	logger.Infof("successfully created cloud sql instance")
	return postgresInstance, croType.StatusMessage(croType.PhaseComplete), nil
}

func (p PostgresProvider) DeletePostgres(ctx context.Context, ps *v1alpha1.Postgres) (croType.StatusMessage, error) {
	logger := p.Logger.WithField("action", "DeletePostgres")

	// Build cloudSQL service
	sqladminService, err := sqladmin.NewService(ctx)
	if err != nil {
		return "error building cloud sql admin service", err
	}

	instances, err := getCloudSQLInstances(sqladminService)
	if err != nil {
		return "Cannot retrieve sql instances from gcp", err
	}
	logrus.Info("listed sql instances from gcp")

	// TODO - Not the best idea - can always fail here if postgres cr never got the annotation
	instanceName := annotations.Get(ps, ResourceIdentifierAnnotation)

	if instanceName == "" {
		msg := "unable to find instance name from annotation"
		return croType.StatusMessage(msg), fmt.Errorf(msg)
	}

	// check if instance already exist
	var foundInstance *sqladmin.DatabaseInstance
	for _, instance := range instances {
		if instance.Name == instanceName {
			logrus.Infof("found matching intance by name: %s", instanceName)
			foundInstance = instance
			break
		}
	}

	// Delete if found
	if foundInstance != nil {
		// return if delete is in progress
		if foundInstance.State != "RUNNABLE" {
			statusMessage := fmt.Sprintf("delete detected, DeletePostgres() in progress, current cloud sql status is %s", foundInstance.State)
			logger.Info(statusMessage)
			return croType.StatusMessage(statusMessage), nil
		}

		// delete if not in progress
		_, err := sqladminService.Instances.Delete(projectID, instanceName).Context(ctx).Do()
		if err != nil {
			return croType.StatusMessage(fmt.Sprintf("failed to delete cloud sql instance: %s", instanceName)), err
		}

		logrus.Info("Triggered deleteDBInstance()")
		return "delete detected, deleteDBInstance() started", nil
	}

	// delete credential secret
	logger.Info("deleting rds secret")
	sec := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ps.Name + defaultCredSecSuffix,
			Namespace: ps.Namespace,
		},
	}
	err = p.Client.Delete(ctx, sec)
	if err != nil && !k8serr.IsNotFound(err) {
		msg := "failed to deleted rds secrets"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}

	resources.RemoveFinalizer(&ps.ObjectMeta, DefaultFinalizer)
	if err := p.Client.Update(ctx, ps); err != nil {
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

func (p PostgresProvider) getCloudSQLConfig(ctx context.Context, postgresCR *v1alpha1.Postgres, postgresPass string) (*sqladmin.DatabaseInstance, error) {

	// Get name from annotation and build random resource name as identifier for now
	// Cloud SQL can give the following error if resource name used was used previously in the last week
	// that will affect re-installs
	// Error 409: The Cloud SQL instance already exists. When you delete an instance, you can't reuse the name of the deleted instance until one week from the deletion date., instanceAlreadyExists
	instanceName := annotations.Get(postgresCR, ResourceIdentifierAnnotation)

	if instanceName == "" {
		// TODO - Don't be too random
		name, err := resources.GeneratePassword()
		if err != nil {
			return nil, err
		}
		instanceName = name
	}

	return &sqladmin.DatabaseInstance{
		Project:         projectID,
		Name:            instanceName,
		DatabaseVersion: defaultGCPDatabaseVersion,
		Settings: &sqladmin.Settings{
			Tier: defaultGCPDBInstanceTier,
			IpConfiguration: &sqladmin.IpConfiguration{
				Ipv4Enabled: true,
				AuthorizedNetworks: []*sqladmin.AclEntry{
					{
						Value: "0.0.0.0/0", // TODO - Don't make it all public
					},
				},
			},
		},
		RootPassword: postgresPass,
	}, nil
}

func buildDefaultCloudSQLSecret(ps *v1alpha1.Postgres) *v1.Secret {
	password, err := resources.GeneratePassword()
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ps.Name + defaultCredSecSuffix,
			Namespace: ps.Namespace,
		},
		StringData: map[string]string{
			defaultPostgresUserKey:     defaultGCPPostgresUser,
			defaultPostgresPasswordKey: password,
		},
		Type: v1.SecretTypeOpaque,
	}
}

func fromDatabaseInstance(instance *sqladmin.DatabaseInstance, postgresPass string) (*providers.PostgresInstance, error) {
	return &providers.PostgresInstance{
		DeploymentDetails: &providers.PostgresDeploymentDetails{
			Username: defaultPostgresUserKey,
			Password: postgresPass,
			Host:     instance.IpAddresses[0].IpAddress,
			Database: defaultGCPPostgresUser,
			Port:     defaultGCPPostgresPort, // Can't be changed on Cloud SQL
		},
	}, nil
}
