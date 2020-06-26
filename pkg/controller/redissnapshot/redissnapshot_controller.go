package redissnapshot

import (
	"context"
	"fmt"

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

const (
	redisProviderName = "aws-elasticache"
)

// Add creates a new RedisSnapshot Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_redis_snapshot"})
	provider := croAws.NewAWSRedisSnapshotProvider(mgr.GetClient(), logger)
	return &ReconcileRedisSnapshot{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		logger:            logger,
		provider:          provider,
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
	provider          providers.RedisSnapshotProvider
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

	// generate info metrics
	defer r.exposeRedisSnapshotMetrics(ctx, instance)

	// get redis cr
	redisCr := &integreatlyv1alpha1.Redis{}
	err = r.client.Get(ctx, types.NamespacedName{Name: instance.Spec.ResourceName, Namespace: instance.Namespace}, redisCr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get redis cr : %s", err.Error())
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
		}
		return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, errorUtil.New(errMsg)
	}

	// check redis cr deployment type is aws
	if !r.provider.SupportsStrategy(redisCr.Status.Strategy) {
		errMsg := fmt.Sprintf("the resource %s uses an unsupported provider strategy %s, only resources using the aws provider are valid", instance.Spec.ResourceName, redisCr.Status.Strategy)
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
		}
		return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, errorUtil.New(errMsg)
	}

	if instance.DeletionTimestamp != nil {
		msg, err := r.provider.DeleteRedisSnapshot(ctx, instance, redisCr)
		if err != nil {
			if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
				return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
			}
			return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, errorUtil.Wrapf(err, "failed to delete redis snapshot")
		}

		r.logger.Info("waiting on redis snapshot to successfully delete")
		if err = resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseDeleteInProgress, msg); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true, RequeueAfter: resources.SuccessReconcileTime}, nil
	}

	// check status, if complete return
	if instance.Status.Phase == croType.PhaseComplete {
		r.logger.Infof("skipping creation of snapshot for %s as phase is complete", instance.Name)
		return reconcile.Result{Requeue: true, RequeueAfter: resources.SuccessReconcileTime}, nil
	}

	// create the snapshot and return the phase
	snap, msg, err := r.provider.CreateRedisSnapshot(ctx, instance, redisCr)

	// error trying to create snapshot
	if err != nil {
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseFailed, msg); updateErr != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
		}
		return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, err
	}

	// no error but the snapshot doesn't exist yet
	if snap == nil {
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseInProgress, msg); updateErr != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
		}
		return reconcile.Result{Requeue: true, RequeueAfter: resources.SuccessReconcileTime}, nil
	}

	// no error, snapshot exists
	if updateErr := resources.UpdateSnapshotPhase(ctx, r.client, instance, croType.PhaseComplete, msg); updateErr != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: resources.ErrorReconcileTime}, updateErr
	}
	return reconcile.Result{Requeue: true, RequeueAfter: resources.SuccessReconcileTime}, nil
}

func buildRedisSnapshotStatusMetricLabels(cr *integreatlyv1alpha1.RedisSnapshot, clusterID, snapshotName string, phase croType.StatusPhase) map[string]string {
	labels := map[string]string{}
	labels["clusterID"] = clusterID
	labels["resourceID"] = cr.Name
	labels["namespace"] = cr.Namespace
	labels["instanceID"] = snapshotName
	labels["productName"] = cr.Labels["productName"]
	labels["strategy"] = redisProviderName
	labels["statusPhase"] = string(phase)
	return labels
}

func (r *ReconcileRedisSnapshot) exposeRedisSnapshotMetrics(ctx context.Context, cr *integreatlyv1alpha1.RedisSnapshot) {
	// build instance name
	snapshotName := cr.Status.SnapshotID

	// get Cluster Id
	logrus.Info("setting redis snapshot information metric")
	clusterID, err := resources.GetClusterID(ctx, r.client)
	if err != nil {
		logrus.Errorf("failed to get cluster id while exposing information metric for %v", snapshotName)
		return
	}

	// set generic status metrics
	// a single metric should be exposed for each possible phase
	// the value of the metric should be 1.0 when the resource is in that phase
	// the value of the metric should be 0.0 when the resource is not in that phase
	// this follows the approach that pod status
	for _, phase := range []croType.StatusPhase{croType.PhaseFailed, croType.PhaseDeleteInProgress, croType.PhasePaused, croType.PhaseComplete, croType.PhaseInProgress} {
		labelsFailed := buildRedisSnapshotStatusMetricLabels(cr, clusterID, snapshotName, phase)
		resources.SetMetric(resources.DefaultRedisSnapshotStatusMetricName, labelsFailed, resources.Btof64(cr.Status.Phase == phase))
	}
}
