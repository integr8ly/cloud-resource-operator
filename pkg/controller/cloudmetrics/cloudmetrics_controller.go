package cloudmetrics

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

var log = logf.Log.WithName("controller_cloudmetrics")

// Set the reconcile duration for this controller.
// Currently it will be called once every 5 minutes
const watchDuration = 600

// Add creates a new CloudMetrics Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCloudMetrics{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cloudmetrics-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Set up a GenericEvent channel that will be used
	// as the event source to trigger the controller's
	// reconcile loop
	events := make(chan event.GenericEvent)

	// Send a generic event to the channel to kick
	// off the first reconcile loop
	go func() {
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

// blank assignment to verify that ReconcileCloudMetrics implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCloudMetrics{}

// ReconcileCloudMetrics reconciles a CloudMetrics object
type ReconcileCloudMetrics struct {
	client client.Client
	scheme *runtime.Scheme
}

// This controller reconciles metrics for cloud resources (currently redis and postgres)
// It takes a sync the world approach, reconciling all cloud resources every set period
// of time (currently every 5 minutes)
func (r *ReconcileCloudMetrics) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CloudMetrics")

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

	// Requeue every 5 minutes
	return reconcile.Result{
		RequeueAfter: watchDuration * time.Second,
	}, nil
}
