package redis

import (
	"context"
	"fmt"
	"time"

	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/openshift"
	errorUtil "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_redis")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Redis Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRedis{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("redis-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Redis
	err = c.Watch(&source.Kind{Type: &integreatlyv1alpha1.Redis{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Redis
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &integreatlyv1alpha1.Redis{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRedis implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRedis{}

// ReconcileRedis reconciles a Redis object
type ReconcileRedis struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Redis object and makes changes based on the state read
// and what is in the Redis.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRedis) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	providerList := []providers.RedisProvider{aws.NewAWSRedisProvider(r.client), openshift.NewOpenShiftRedisProvider(r.client)}

	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Redis")
	ctx := context.TODO()
	cfgMgr := providers.NewConfigManager(providers.DefaultProviderConfigMapName, providers.DefaultConfigNamespace, r.client)

	// Fetch the Redis instance
	instance := &integreatlyv1alpha1.Redis{}
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

	stratMap, err := cfgMgr.GetStrategyMappingForDeploymentType(ctx, instance.Spec.Type)
	if err != nil {
		return reconcile.Result{}, errorUtil.Wrapf(err, "failed to read deployment type config for deployment %s", instance.Spec.Type)
	}

	for _, p := range providerList {
		if p.SupportsStrategy(stratMap.Redis) {
			// handle deletion of redis and remove any finalizers added
			if instance.GetDeletionTimestamp() != nil {
				err := p.DeleteRedis(ctx, instance)
				if err != nil {
					return reconcile.Result{}, errorUtil.Wrapf(err, "failed to perform provider specific cluster deletion")
				}
				return reconcile.Result{}, nil
			}

			// handle creation of redis and apply any finalizers to instance required for deletion
			redis, err := p.CreateRedis(ctx, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			if redis == nil {
				reqLogger.Info("waiting for redis cluster to become available")
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
			}

			// create the secret with the redis cluster connection details
			sec := &corev1.Secret{
				ObjectMeta: controllerruntime.ObjectMeta{
					Name:      instance.Spec.SecretRef.Name,
					Namespace: instance.Namespace,
				},
			}
			reqLogger.Info("creating or updating client secret")
			_, err = controllerruntime.CreateOrUpdate(ctx, r.client, sec, func(existing runtime.Object) error {
				e := existing.(*corev1.Secret)
				if err = controllerutil.SetControllerReference(instance, e, r.scheme); err != nil {
					return errorUtil.Wrapf(err, "failed to set owner on secret %s", sec.Name)
				}
				e.Data = redis.DeploymentDetails.Data()
				e.Type = corev1.SecretTypeOpaque
				return nil
			})
			if err != nil {
				return reconcile.Result{}, err
			}

			// update the redis custom resource
			instance.Status.SecretRef = instance.Spec.SecretRef
			instance.Status.Strategy = stratMap.Redis
			instance.Status.Provider = p.GetName()
			if err = r.client.Status().Update(ctx, instance); err != nil {
				return reconcile.Result{}, errorUtil.Wrapf(err, "failed to update instance %s in namespace %s", instance.Name, instance.Namespace)
			}

			return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
		}
	}
	return reconcile.Result{}, errorUtil.New(fmt.Sprintf("unsupported deployment strategy %s", stratMap.Redis))
}
