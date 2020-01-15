package redissnapshot

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	croType "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	croAws "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	errorUtil "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new RedisSnapshot Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_redis_snapshot"})
	return &ReconcileRedisSnapshot{
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
	c, err := controller.New("redissnapshot-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource RedisSnapshot
	err = c.Watch(&source.Kind{Type: &integreatlyv1alpha1.RedisSnapshot{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner RedisSnapshot
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &integreatlyv1alpha1.RedisSnapshot{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRedisSnapshot implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRedisSnapshot{}

// ReconcileRedisSnapshot reconciles a RedisSnapshot object
type ReconcileRedisSnapshot struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client            client.Client
	scheme            *runtime.Scheme
	logger            *logrus.Entry
	ConfigManager     croAws.ConfigManager
	CredentialManager croAws.CredentialManager
}

// Reconcile reads that state of the cluster for a RedisSnapshot object and makes changes based on the state read
// and what is in the RedisSnapshot.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRedisSnapshot) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.logger.Info("reconciling redis snapshot")
	ctx := context.TODO()

	// Fetch the RedisSnapshot instance
	instance := &integreatlyv1alpha1.RedisSnapshot{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
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
		r.logger.Infof("skipping creation of snapshot for %s as phase is complete", instance.Name)
		return reconcile.Result{}, nil
	}

	// get redis cr
	redisCr := &integreatlyv1alpha1.Redis{}
	err = r.client.Get(ctx, types.NamespacedName{Name: instance.Spec.ResourceName, Namespace: instance.Namespace}, redisCr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get redis cr : %s", err.Error())
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
	}

	// check redis cr deployment type is aws
	if redisCr.Status.Strategy != providers.AWSDeploymentStrategy {
		errMsg := fmt.Sprintf("the resource %s uses an unsupported provider strategy %s, only resources using the aws provider are valid", instance.Spec.ResourceName, redisCr.Status.Strategy)
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, errorUtil.New(errMsg)
	}

	// get resource region
	stratCfg, err := r.ConfigManager.ReadStorageStrategy(ctx, providers.RedisResourceType, redisCr.Spec.Tier)
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
	providerCreds, err := r.CredentialManager.ReconcileProviderCredentials(ctx, redisCr.Namespace)
	if err != nil {
		errMsg := "failed to reconcile elasticache credentials"
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, errorUtil.Wrap(err, errMsg)
	}

	// setup aws elasticache cluster sdk session
	cacheSvc := elasticache.New(session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(stratCfg.Region),
		Credentials: credentials.NewStaticCredentials(providerCreds.AccessKeyID, providerCreds.SecretAccessKey, ""),
	})))

	// create snapshot of primary node
	phase, msg, err := r.createSnapshot(ctx, cacheSvc, instance, redisCr)
	if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, phase, msg); updateErr != nil {
		return reconcile.Result{}, updateErr
	}
	if err != nil {
		return reconcile.Result{}, err
	}

return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 60}, nil
}

func (r *ReconcileRedisSnapshot) createSnapshot(ctx context.Context, cacheSvc elasticacheiface.ElastiCacheAPI, snapshot *integreatlyv1alpha1.RedisSnapshot, redis *integreatlyv1alpha1.Redis) (croType.StatusPhase, croType.StatusMessage, error) {
	// generate snapshot name
	snapshotName, err := croAws.BuildTimestampedInfraNameFromObjectCreation(ctx, r.client, snapshot.ObjectMeta, croAws.DefaultAwsIdentifierLength)
	if err != nil {
		errMsg := "failed to generate snapshot name"
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// update cr with snapshot name
	snapshot.Status.SnapshotID = snapshotName
	if err = r.client.Status().Update(ctx, snapshot); err != nil {
		errMsg := fmt.Sprintf("failed to update instance %s in namespace %s", snapshot.Name, snapshot.Namespace)
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// generate cluster name
	clusterName, err := croAws.BuildInfraNameFromObject(ctx, r.client, redis.ObjectMeta, croAws.DefaultAwsIdentifierLength)
	if err != nil {
		errMsg := "failed to get cluster name"
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// check snapshot exists
	listOutput, err := cacheSvc.DescribeSnapshots(&elasticache.DescribeSnapshotsInput{
		SnapshotName: aws.String(snapshotName),
	})
	var foundSnapshot *elasticache.Snapshot
	for _, c := range listOutput.Snapshots {
		if *c.SnapshotName == snapshotName {
			foundSnapshot = c
			break
		}
	}

	// get replication group
	cacheOutput, err := cacheSvc.DescribeReplicationGroups(&elasticache.DescribeReplicationGroupsInput{
		ReplicationGroupId: aws.String(clusterName),
	})

	if cacheOutput == nil {
		return croType.PhaseFailed, "snapshot failed, no replication group found", nil
	}

	// ensure replication group is available
	if *cacheOutput.ReplicationGroups[0].Status != "available" {
		errMsg := fmt.Sprintf("current replication group status is %s", *cacheOutput.ReplicationGroups[0].Status)
		return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}

	// find primary cache node
	cacheName := ""
	for _, i := range cacheOutput.ReplicationGroups[0].NodeGroups[0].NodeGroupMembers {
		if *i.CurrentRole == "primary" {
			cacheName = *i.CacheClusterId
			break
		}
	}

	// create snapshot of primary cache node
	if foundSnapshot == nil {
		r.logger.Info("creating elasticache snapshot")
		if _, err = cacheSvc.CreateSnapshot(&elasticache.CreateSnapshotInput{
			CacheClusterId: aws.String(cacheName),
			SnapshotName:   aws.String(snapshotName),
		}); err != nil {
			errMsg := fmt.Sprintf("error creating elasticache snapshot %s", err)
			return croType.PhaseFailed, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}
		return croType.PhaseInProgress, "snapshot started", nil
	}

	// if snapshot status complete update status
	if *foundSnapshot.SnapshotStatus == "available" {
		return croType.PhaseComplete, "snapshot created", nil
	}

	msg := fmt.Sprintf("current snapshot status : %s", *foundSnapshot.SnapshotStatus)
	r.logger.Info(msg)
	return croType.PhaseInProgress, croType.StatusMessage(msg), nil
}
