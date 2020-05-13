package cloudwatchmetrics

import (
	"context"
	"time"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cloudwatchmetrics")

// Set the reconcile duration for this controller.
// Currently it will be called once every 5 minutes
const watchDuration = 5

// Add creates a new CloudwatchMetrics Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCloudwatchMetrics{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cloudwatchmetrics-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Push an event to the channel every 5 minutes to
	// trigger a new reconcile.Request
	events := make(chan event.GenericEvent)
	go func() {
		time.Sleep(watchDuration * time.Minute)
		events <- event.GenericEvent{
			Meta:   &integreatlyv1alpha1.Redis{},
			Object: &integreatlyv1alpha1.Redis{},
		}
	}()

	// Setup the controller to use the channel as its watch source
	err = c.Watch(&source.Channel{Source: events}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileCloudwatchMetrics implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCloudwatchMetrics{}

// ReconcileCloudwatchMetrics reconciles a CloudwatchMetrics object
type ReconcileCloudwatchMetrics struct {
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCloudwatchMetrics) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CloudwatchMetrics")

	// Fetch all redis crs
	redisInstances := &integreatlyv1alpha1.RedisList{}
	err := r.client.List(context.TODO(), redisInstances)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(redisInstances.Items) > 0 {
		for _, redis := range redisInstances.Items {
			reqLogger.Info("Found redis cr:", redis.Name)
		}
	} else {
		reqLogger.Info("Found no redis instances")
	}

	// Fetch all postgres crs
	postgresInstances := &integreatlyv1alpha1.PostgresList{}
	err = r.client.List(context.TODO(), postgresInstances)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(postgresInstances.Items) > 0 {
		for _, postgres := range postgresInstances.Items {
			reqLogger.Info("Found postgres cr:", postgres.Name)
		}
	} else {
		reqLogger.Info("Found no postgres instances")
	}

	return reconcile.Result{}, nil
}
