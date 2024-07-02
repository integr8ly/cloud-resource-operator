// This controller reconciles metrics for cloud resources (currently redis and postgres)
// It takes a sync the world approach, reconciling all cloud resources every set period
// of time (currently every 5 minutes)
package cloudmetrics

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp"

	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	customMetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CroGaugeMetric allows for a mapping between an exposed prometheus metric and multiple cloud provider specific metric
type CroGaugeMetric struct {
	Name         string
	GaugeVec     *prometheus.GaugeVec
	ProviderType map[string]providers.CloudProviderMetricType
}

// postgresGaugeMetrics stores a mapping between an exposed (postgres) prometheus metric and multiple cloud provider specific metric
// to add any addition metrics simply add to this mapping and it will be scraped and exposed
var postgresGaugeMetrics = []CroGaugeMetric{
	{
		Name: resources.PostgresFreeStorageAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.PostgresFreeStorageAverageMetricName,
				Help: "The amount of available storage space. Units: Bytes",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresFreeStorageAverageMetricName,
				ProviderMetricName:   "FreeStorageSpace",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresFreeStorageAverageMetricName,
				ProviderMetricName:   "cloudsql.googleapis.com/database/disk/quota-cloudsql.googleapis.com/database/disk/bytes_used",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.PostgresCPUUtilizationAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.PostgresCPUUtilizationAverageMetricName,
				Help: "The percentage of CPU utilization. Units: Percent",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresCPUUtilizationAverageMetricName,
				ProviderMetricName:   "CPUUtilization",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresCPUUtilizationAverageMetricName,
				ProviderMetricName:   "cloudsql.googleapis.com/database/cpu/utilization",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.PostgresFreeableMemoryAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.PostgresFreeableMemoryAverageMetricName,
				Help: "The amount of available random access memory. Units: Bytes",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresFreeableMemoryAverageMetricName,
				ProviderMetricName:   "FreeableMemory",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresFreeableMemoryAverageMetricName,
				ProviderMetricName:   "cloudsql.googleapis.com/database/memory/quota-cloudsql.googleapis.com/database/memory/total_usage",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.PostgresMaxMemoryMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.PostgresMaxMemoryMetricName,
				Help: "The amount of max random access memory. Units: Bytes",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresMaxMemoryMetricName,
				ProviderMetricName:   "cloudsql.googleapis.com/database/memory/quota",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.PostgresAllocatedStorageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.PostgresAllocatedStorageMetricName,
				Help: "The amount of currently used storage space. Units: Bytes",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.PostgresAllocatedStorageMetricName,
				ProviderMetricName:   "cloudsql.googleapis.com/database/disk/bytes_used",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
}

// redisGaugeMetrics stores a mapping between an exposed (redis) prometheus metric and multiple cloud provider specific metric
// to add any addition metrics simply add to this mapping and it will be scraped and exposed
var redisGaugeMetrics = []CroGaugeMetric{
	{
		Name: resources.RedisMemoryUsagePercentageAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.RedisMemoryUsagePercentageAverageMetricName,
				Help: "The percentage of redis used memory. Units: Percent",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.RedisMemoryUsagePercentageAverageMetricName,
				//calculated on used_memory/maxmemory from Redis INFO http://redis.io/commands/info
				ProviderMetricName: "DatabaseMemoryUsagePercentage",
				Statistic:          cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.RedisMemoryUsagePercentageAverageMetricName,
				ProviderMetricName:   "redis.googleapis.com/stats/memory/usage_ratio",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.RedisFreeableMemoryAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.RedisFreeableMemoryAverageMetricName,
				Help: "The amount of available random access memory. Units: Bytes",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.RedisFreeableMemoryAverageMetricName,
				ProviderMetricName:   "FreeableMemory",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.RedisFreeableMemoryAverageMetricName,
				ProviderMetricName:   "redis.googleapis.com/stats/memory/maxmemory-redis.googleapis.com/stats/memory/usage",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.RedisCPUUtilizationAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.RedisCPUUtilizationAverageMetricName,
				Help: "The percentage of CPU utilization. Units: Percent",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.RedisCPUUtilizationAverageMetricName,
				ProviderMetricName:   "CPUUtilization",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.RedisCPUUtilizationAverageMetricName,
				ProviderMetricName:   "redis.googleapis.com/stats/cpu_utilization",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
	{
		Name: resources.RedisEngineCPUUtilizationAverageMetricName,
		GaugeVec: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: resources.RedisEngineCPUUtilizationAverageMetricName,
				Help: "The percentage of CPU utilization. Units: Percent",
			},
			genericMetricLabelNames()),
		ProviderType: map[string]providers.CloudProviderMetricType{
			providers.AWSDeploymentStrategy: {
				PrometheusMetricName: resources.RedisEngineCPUUtilizationAverageMetricName,
				ProviderMetricName:   "EngineCPUUtilization",
				Statistic:            cloudwatch.StatisticAverage,
			},
			providers.GCPDeploymentStrategy: {
				PrometheusMetricName: resources.RedisEngineCPUUtilizationAverageMetricName,
				ProviderMetricName:   "redis.googleapis.com/stats/cpu_utilization_main_thread",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
	},
}

// PostgresReconciler reconciles a Postgres object
type CloudMetricsReconciler struct {
	k8sclient.Client
	scheme               *runtime.Scheme
	logger               *logrus.Entry
	postgresProviderList []providers.PostgresMetricsProvider
	redisProviderList    []providers.RedisMetricsProvider
}

// blank assignment to verify that ReconcileCloudMetrics implements reconcile.Reconciler
var _ reconcile.Reconciler = &CloudMetricsReconciler{}

// New returns a new reconcile.Reconciler
func New(mgr manager.Manager) (*CloudMetricsReconciler, error) {
	restConfig := ctrl.GetConfigOrDie()
	restConfig.Timeout = time.Second * 10
	client, err := k8sclient.New(restConfig, k8sclient.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return nil, err
	}
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_cloudmetrics"})
	awsPostgresMetricsProvider, err := aws.NewAWSPostgresMetricsProvider(client, logger)
	if err != nil {
		return nil, err
	}
	gcpPostgresMetricsProvider, err := gcp.NewGCPPostgresMetricsProvider(client, logger)
	if err != nil {
		return nil, err
	}
	postgresProviderList := []providers.PostgresMetricsProvider{
		awsPostgresMetricsProvider,
		gcpPostgresMetricsProvider,
	}
	awsRedisMetricsProvider, err := aws.NewAWSRedisMetricsProvider(client, logger)
	if err != nil {
		return nil, err
	}
	gcpRedisMetricsProvider, err := gcp.NewGCPRedisMetricsProvider(client, logger)
	if err != nil {
		return nil, err
	}
	redisProviderList := []providers.RedisMetricsProvider{
		awsRedisMetricsProvider,
		gcpRedisMetricsProvider,
	}

	// we only wish to register metrics once when the new reconciler is created
	// as the metrics we want to expose are known in advance we can register them all
	// they will only be exposed if there is a value returned for the vector for a provider
	registerGaugeVectorMetrics(logger)
	return &CloudMetricsReconciler{
		Client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		logger:               logger,
		postgresProviderList: postgresProviderList,
		redisProviderList:    redisProviderList,
	}, nil
}

func (r *CloudMetricsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up a GenericEvent channel that will be used
	// as the event source to trigger the controller's
	// reconcile loop
	events := make(chan event.GenericEvent)

	// Send a generic event to the channel to kick
	// off the first reconcile loop
	go func() {
		events <- event.GenericEvent{
			Object: &integreatlyv1alpha1.Redis{},
		}
	}()

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&integreatlyv1alpha1.Redis{}).
		Build(r)
	if err != nil {
		return err
	}
	err = c.Watch(
		&source.Channel{Source: events},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *CloudMetricsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("reconciling CloudMetrics")

	// scrapedMetrics stores the GenericCloudMetric which are returned from the providers
	var scrapedMetrics []*providers.GenericCloudMetric

	// fetch all redis crs
	redisInstances := &integreatlyv1alpha1.RedisList{}
	err := r.Client.List(ctx, redisInstances)
	if err != nil {
		return ctrl.Result{}, err
	}

	// loop through the redis crs and scrape the related provider specific metrics
	for index := range redisInstances.Items {
		redis := redisInstances.Items[index]
		r.logger.Infof("beginning to scrape metrics for redis cr: %s", redis.Name)
		for _, p := range r.redisProviderList {
			// only scrape metrics on supported strategies
			if !p.SupportsStrategy(redis.Status.Strategy) {
				continue
			}
			var redisMetricTypes []providers.CloudProviderMetricType
			for _, gaugeMetric := range redisGaugeMetrics {
				for provider, metricType := range gaugeMetric.ProviderType {
					if provider == redis.Status.Strategy {
						redisMetricTypes = append(redisMetricTypes, metricType)
						continue
					}
				}
			}

			// all redis scrapedMetric providers inherit the same interface
			// scrapeMetrics returns scraped metrics output which contains a list of GenericCloudMetrics
			scrapedMetricsOutput, err := p.ScrapeRedisMetrics(ctx, &redis, redisMetricTypes)
			if err != nil {
				r.logger.Errorf("failed to scrape metrics for redis: %v", err)
				continue
			}

			scrapedMetrics = append(scrapedMetrics, scrapedMetricsOutput.Metrics...)
		}
	}
	// for each scraped metric value we check redisGaugeMetrics for a match and set the value and labels
	r.setGaugeMetrics(redisGaugeMetrics, scrapedMetrics)

	// Fetch all postgres crs
	postgresInstances := &integreatlyv1alpha1.PostgresList{}
	err = r.Client.List(ctx, postgresInstances)
	if err != nil {
		r.logger.Error(err)
	}
	for index := range postgresInstances.Items {
		postgres := postgresInstances.Items[index]
		r.logger.Infof("beginning to scrape metrics for postgres cr: %s", postgres.Name)
		for _, p := range r.postgresProviderList {
			// only scrape metrics on supported strategies
			if !p.SupportsStrategy(postgres.Status.Strategy) {
				continue
			}

			// filter out the provider specific metric from the postgresGaugeMetrics map which defines the metrics we want to scrape
			var postgresMetricTypes []providers.CloudProviderMetricType
			for _, gaugeMetric := range postgresGaugeMetrics {
				for provider, metricType := range gaugeMetric.ProviderType {
					if provider == postgres.Status.Strategy {
						postgresMetricTypes = append(postgresMetricTypes, metricType)
						continue
					}
				}
			}

			// all postgres scrapedMetric providers inherit the same interface
			// scrapeMetrics returns scraped metrics output which contains a list of GenericCloudMetrics
			scrapedMetricsOutput, err := p.ScrapePostgresMetrics(ctx, &postgres, postgresMetricTypes)
			if err != nil {
				r.logger.Errorf("failed to scrape metrics for postgres %v", err)
				continue
			}

			// add the returned scraped metrics to the list of metrics
			scrapedMetrics = append(scrapedMetrics, scrapedMetricsOutput.Metrics...)
		}
	}

	// for each scraped metric value we check postgresGaugeMetrics for a match and set the value and labels
	r.setGaugeMetrics(postgresGaugeMetrics, scrapedMetrics)

	// we want full control over when we scrape metrics
	// to allow for this we only have a single requeue
	// this ensures regardless of errors or return times
	// all metrics are scraped and exposed at the same time
	return ctrl.Result{
		RequeueAfter: resources.GetMetricReconcileTimeOrDefault(resources.MetricsWatchDuration),
	}, nil
}

func registerGaugeVectorMetrics(logger *logrus.Entry) {
	for _, metric := range postgresGaugeMetrics {
		logger.Infof("registering metric: %s ", metric.Name)
		customMetrics.Registry.MustRegister(metric.GaugeVec)
		resources.MetricVecs[metric.Name] = *metric.GaugeVec
	}
	for _, metric := range redisGaugeMetrics {
		logger.Infof("registering metric: %s ", metric.Name)
		customMetrics.Registry.MustRegister(metric.GaugeVec)
		resources.MetricVecs[metric.Name] = *metric.GaugeVec
	}
}

// func setGaugeMetrics sets the value on exposed metrics with labels
func (r *CloudMetricsReconciler) setGaugeMetrics(gaugeMetrics []CroGaugeMetric, scrapedMetrics []*providers.GenericCloudMetric) {
	for _, scrapedMetric := range scrapedMetrics {
		for _, croMetric := range gaugeMetrics {
			if scrapedMetric.Name == croMetric.Name {
				croMetric.GaugeVec.WithLabelValues(
					scrapedMetric.Labels[resources.LabelClusterIDKey],
					scrapedMetric.Labels[resources.LabelResourceIDKey],
					scrapedMetric.Labels[resources.LabelNamespaceKey],
					scrapedMetric.Labels[resources.LabelInstanceIDKey],
					scrapedMetric.Labels[resources.LabelProductNameKey],
					scrapedMetric.Labels[resources.LabelStrategyKey]).Set(scrapedMetric.Value)
				r.logger.Infof("successfully set metric value for %s", croMetric.Name)
				continue
			}
		}
	}
}

func genericMetricLabelNames() []string {
	return []string{
		resources.LabelClusterIDKey,
		resources.LabelResourceIDKey,
		resources.LabelNamespaceKey,
		resources.LabelInstanceIDKey,
		resources.LabelProductNameKey,
		resources.LabelStrategyKey,
	}
}
