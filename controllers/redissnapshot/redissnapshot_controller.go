/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package redissnapshot

import (
	"context"
	"fmt"
	"time"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	croAws "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	errorUtil "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	redisProviderName = "aws-elasticache"
)

// RedisSnapshotReconciler reconciles a RedisSnapshot object
type RedisSnapshotReconciler struct {
	k8sclient.Client
	scheme            *runtime.Scheme
	logger            *logrus.Entry
	provider          providers.RedisSnapshotProvider
	ConfigManager     croAws.ConfigManager
	CredentialManager croAws.CredentialManager
}

func New(mgr manager.Manager) (*RedisSnapshotReconciler, error) {
	restConfig := controllerruntime.GetConfigOrDie()
	restConfig.Timeout = time.Second * 10

	client, err := k8sclient.New(restConfig, k8sclient.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return nil, err
	}
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_redis_snapshot"})
	provider := croAws.NewAWSRedisSnapshotProvider(client, logger)
	return &RedisSnapshotReconciler{
		Client:            client,
		scheme:            mgr.GetScheme(),
		logger:            logger,
		provider:          provider,
		ConfigManager:     croAws.NewDefaultConfigMapConfigManager(mgr.GetClient()),
		CredentialManager: croAws.NewCredentialManager(mgr.GetClient()),
	}, nil
}

func (r *RedisSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&integreatlyv1alpha1.RedisSnapshot{}).
		Watches(&source.Kind{Type: &integreatlyv1alpha1.RedisSnapshot{}}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &integreatlyv1alpha1.RedisSnapshot{},
		}).
		Complete(r)
}

func (r *RedisSnapshotReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("reconciling redis snapshot")
	ctx := context.TODO()

	// Fetch the RedisSnapshot instance
	instance := &integreatlyv1alpha1.RedisSnapshot{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// generate info metrics
	defer r.exposeRedisSnapshotMetrics(ctx, instance)

	// get redis cr
	redisCr := &integreatlyv1alpha1.Redis{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: instance.Spec.ResourceName, Namespace: instance.Namespace}, redisCr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get redis cr : %s", err.Error())
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, errorUtil.New(errMsg)
	}

	// check redis cr deployment type is aws
	if !r.provider.SupportsStrategy(redisCr.Status.Strategy) {
		errMsg := fmt.Sprintf("the resource %s uses an unsupported provider strategy %s, only resources using the aws provider are valid", instance.Spec.ResourceName, redisCr.Status.Strategy)
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseFailed, croType.StatusMessage(errMsg)); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, errorUtil.New(errMsg)
	}

	if instance.DeletionTimestamp != nil {
		msg, err := r.provider.DeleteRedisSnapshot(ctx, instance, redisCr)
		if err != nil {
			if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, errorUtil.Wrapf(err, "failed to delete redis snapshot")
		}

		r.logger.Info("waiting on redis snapshot to successfully delete")
		if err = resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseDeleteInProgress, msg); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: r.provider.GetReconcileTime(instance)}, nil
	}

	// check status, if complete return
	if instance.Status.Phase == croType.PhaseComplete {
		r.logger.Infof("skipping creation of snapshot for %s as phase is complete", instance.Name)
		return ctrl.Result{Requeue: true, RequeueAfter: r.provider.GetReconcileTime(instance)}, nil
	}

	// create the snapshot and return the phase
	snap, msg, err := r.provider.CreateRedisSnapshot(ctx, instance, redisCr)

	// error trying to create snapshot
	if err != nil {
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseFailed, msg); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// no error but the snapshot doesn't exist yet
	if snap == nil {
		if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseInProgress, msg); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{Requeue: true, RequeueAfter: r.provider.GetReconcileTime(instance)}, nil
	}

	// no error, snapshot exists
	if updateErr := resources.UpdateSnapshotPhase(ctx, r.Client, instance, croType.PhaseComplete, msg); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{Requeue: true, RequeueAfter: r.provider.GetReconcileTime(instance)}, nil
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

func (r *RedisSnapshotReconciler) exposeRedisSnapshotMetrics(ctx context.Context, cr *integreatlyv1alpha1.RedisSnapshot) {
	// build instance name
	snapshotName := cr.Status.SnapshotID

	// get Cluster Id
	logrus.Info("setting redis snapshot information metric")
	clusterID, err := resources.GetClusterID(ctx, r.Client)
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
