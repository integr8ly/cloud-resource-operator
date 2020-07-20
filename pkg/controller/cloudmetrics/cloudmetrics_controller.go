// This controller reconciles metrics for cloud resources (currently redis and postgres)
// It takes a sync the world approach, reconciling all cloud resources every set period
// of time (currently every 5 minutes)
package cloudmetrics

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add creates a new CloudMetrics Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	c := mgr.GetClient()
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_cloudmetrics"})
	postgresProviderList := []providers.PostgresMetricsProvider{aws.NewAWSPostgresMetricsProvider(c, logger)}
	redisProviderList := []providers.RedisMetricsProvider{aws.NewAWSRedisMetricsProvider(c, logger)}
	return &ReconcileCloudMetrics{
		client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		logger:               logger,
		postgresProviderList: postgresProviderList,
		redisProviderList:    redisProviderList,
	}
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
	client               client.Client
	scheme               *runtime.Scheme
	logger               *logrus.Entry
	postgresProviderList []providers.PostgresMetricsProvider
	redisProviderList    []providers.RedisMetricsProvider
}

// Reconcile reads all redis and postgres crs periodically and reconcile metrics for these
// resources.
// The Controller will requeue the Request every 5 minutes constantly when RequeueAfter is set
func (r *ReconcileCloudMetrics) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.logger.Info("reconciling CloudMetrics")
	ctx := context.Background()

	// Fetch all redis crs
	redisInstances := &integreatlyv1alpha1.RedisList{}
	err := r.client.List(ctx, redisInstances)
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, redis := range redisInstances.Items {
		r.logger.Infof("beginning to reconcile cloud metrics for redis cr: %s", redis.Name)
		for _, p := range r.redisProviderList {
			// only scrape metrics on supported strategies
			if !p.SupportsStrategy(redis.Status.Strategy) {
				continue
			}

			// all redis metric providers inherit the same interface
			// scrapeMetrics returns scraped metrics output which contains a list of GenericCloudMetrics
			scrapedMetricsOutput, err := p.ScrapeRedisMetrics(ctx, &redis)
			if err != nil {
				r.logger.Errorf("failed to scrape metrics for redis %v", err)
				continue
			}

			for _, metricData := range scrapedMetricsOutput.Metrics {
				r.logger.Debug(*metricData)
			}
		}
	}

	// Fetch all postgres crs
	postgresInstances := &integreatlyv1alpha1.PostgresList{}
	err = r.client.List(ctx, postgresInstances)
	if err != nil {
		r.logger.Error(err)
	}
	for _, postgres := range postgresInstances.Items {
		r.logger.Infof("beginning to reconcile cloud metrics for postgres cr: %s", postgres.Name)
		for _, p := range r.postgresProviderList {
			// only scrape metrics on supported strategies
			if !p.SupportsStrategy(postgres.Status.Strategy) {
				continue
			}

			// all postgres metric providers inherit the same interface
			// scrapeMetrics returns scraped metrics output which contains a list of GenericCloudMetrics
			scrapedMetricsOutput, err := p.ScrapePostgresMetrics(ctx, &postgres)
			if err != nil {
				r.logger.Errorf("failed to scrape metrics for postgres %v", err)
				continue
			}

			for _, metricData := range scrapedMetricsOutput.Metrics {
				r.logger.Debug(*metricData)
			}
		}
	}

	// we want full control over when we scrape metrics
	// to allow for this we only have a single requeue
	// this ensures regardless of errors or return times
	// all metrics are scraped and exposed at the same time
	return reconcile.Result{
		RequeueAfter: resources.GetMetricReconcileTimeOrDefault(resources.MetricsWatchDuration),
	}, nil
}
