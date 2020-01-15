package postgressnapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/rds/rdsiface"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	croAws "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new PostgresSnapshot Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_postgres_snapshot"})
	return &ReconcilePostgresSnapshot{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		logger:            logger,
		ConfigManager:     croAws.NewDefaultConfigMapConfigManager(mgr.GetClient()),
		CredentialManager: croAws.NewCredentialMinterCredentialManager(mgr.GetClient()),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("postgressnapshot-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PostgresSnapshot
	err = c.Watch(&source.Kind{Type: &integreatlyv1alpha1.PostgresSnapshot{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner PostgresSnapshot
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &integreatlyv1alpha1.PostgresSnapshot{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePostgresSnapshot implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePostgresSnapshot{}

// ReconcilePostgresSnapshot reconciles a PostgresSnapshot object
type ReconcilePostgresSnapshot struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client            client.Client
	scheme            *runtime.Scheme
	logger            *logrus.Entry
	ConfigManager     croAws.ConfigManager
	CredentialManager croAws.CredentialManager
}

// Reconcile reads that state of the cluster for a PostgresSnapshot object and makes changes based on the state read
// and what is in the PostgresSnapshot.Spec
func (r *ReconcilePostgresSnapshot) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.logger.Info("reconciling postgres snapshot")
	ctx := context.TODO()

	// Fetch the PostgresSnapshot instance
	instance := &integreatlyv1alpha1.PostgresSnapshot{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// check status, if complete return
	if instance.Status.Phase == croType.PhaseComplete {
		r.logger.Infof("found existing rds snapshot for %s", instance.Name)
		return reconcile.Result{}, nil
	}

	// get postgres cr
	postgresCr := &integreatlyv1alpha1.Postgres{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.ResourceName, Namespace: instance.Namespace}, postgresCr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get postgres resource: %s", err.Error())
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, errorUtil.New(errMsg)
	}

	// check postgres deployment strategy is aws
	if postgresCr.Status.Strategy != providers.AWSDeploymentStrategy {
		errMsg := fmt.Sprintf("deployment strategy '%s' is not supported", postgresCr.Status.Strategy)
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, errorUtil.New(errMsg)
	}

	// get resource region
	stratCfg, err := r.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, postgresCr.Spec.Tier)
	if err != nil {
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(err.Error())); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, err
	}
	if stratCfg.Region == "" {
		stratCfg.Region = croAws.DefaultRegion
	}

	// create the credentials to be used by the aws resource providers, not to be used by end-user
	providerCreds, err := r.CredentialManager.ReconcileProviderCredentials(ctx, postgresCr.Namespace)
	if err != nil {
		errMsg := "failed to reconcile rds credentials"
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, errorUtil.Wrap(err, errMsg)
	}

	// setup aws rds session
	rdsSvc := rds.New(session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	})))

	// create the snapshot and return the phase
	phase, msg, err := r.createSnapshot(ctx, rdsSvc, instance, postgresCr)
	if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, phase, msg); updateErr != nil {
		return reconcile.Result{}, updateErr
	}
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 60}, nil
}

func (r *ReconcilePostgresSnapshot) createSnapshot(ctx context.Context, rdsSvc rdsiface.RDSAPI, snapshot *integreatlyv1alpha1.PostgresSnapshot, postgres *integreatlyv1alpha1.Postgres) (croType.StatusPhase, croType.StatusMessage, error) {
	// generate snapshot name
	snapshotName, err := croAws.BuildTimestampedInfraNameFromObjectCreation(ctx, r.client, snapshot.ObjectMeta, croAws.DefaultAwsIdentifierLength)
	if err != nil {
		errMsg := "failed to generate snapshot name"
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// get instance name
	instanceName, err := croAws.BuildInfraNameFromObject(ctx, r.client, postgres.ObjectMeta, croAws.DefaultAwsIdentifierLength)
	if err != nil {
		errMsg := "failed to get cluster name"
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// check snapshot exists
	listOutput, err := rdsSvc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{
		DBSnapshotIdentifier: aws.String(snapshotName),
	})
	var foundSnapshot *rds.DBSnapshot
	for _, c := range listOutput.DBSnapshots {
		if *c.DBSnapshotIdentifier == snapshotName {
			foundSnapshot = c
			break
		}
	}

	// create snapshot of the rds instance
	if foundSnapshot == nil {
		r.logger.Info("creating rds snapshot")
		_, err = rdsSvc.CreateDBSnapshot(&rds.CreateDBSnapshotInput{
			DBInstanceIdentifier: aws.String(instanceName),
			DBSnapshotIdentifier: aws.String(snapshotName),
		})
		if err != nil {
			errMsg := "error creating rds snapshot"
			return croType.PhaseFailed, croType.StatusMessage(fmt.Sprintf("error creating rds snapshot %s", errMsg)), errorUtil.Wrap(err, errMsg)
		}
		return croType.PhaseInProgress, "snapshot started", nil
	}

	// if snapshot status complete update status
	if *foundSnapshot.Status == "available" {
		return croType.PhaseComplete, "snapshot created", nil
	}

	// creation in progress
	return croType.PhaseInProgress, "snapshot creation in progress", nil
}
