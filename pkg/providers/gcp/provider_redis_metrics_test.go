package gcp

import (
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"context"
	"fmt"
	"github.com/googleapis/gax-go/v2"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"google.golang.org/protobuf/types/known/timestamppb"
	"reflect"
	"testing"
	"time"
)

var (
	testInterval = &monitoringpb.TimeInterval{
		StartTime: timestamppb.New(time.Now().Add(-resources.GetMetricReconcileTimeOrDefault(resources.MetricsWatchDuration))),
		EndTime:   timestamppb.Now(),
	}
	testPoint = &monitoringpb.Point{
		Interval: testInterval,
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{
				DoubleValue: 0.5,
			},
		},
	}
)

func Test_getMetricData(t *testing.T) {
	type args struct {
		metricClient gcpiface.MetricApi
		opts         getMetricDataOpts
	}
	opts := getMetricDataOpts{
		metric: providers.CloudProviderMetricType{
			PrometheusMetricName: resources.RedisMemoryUsagePercentageAverage,
			ProviderMetricName:   "redis.googleapis.com/stats/memory/usage_ratio",
			Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
		},
		projectID: gcpTestProjectId,
		filter:    fmt.Sprintf(redisMetricFilterTemplate, resources.MonitoringResourceTypeRedisInstance, gcpTestRedisInstanceName, "redis.googleapis.com/stats/memory/usage_ratio"),
		interval:  testInterval,
	}
	tests := []struct {
		name    string
		args    args
		want    *providers.GenericCloudMetric
		wantErr bool
	}{
		{
			name: "success getting metric data",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
				}),
				opts: opts,
			},
			want: &providers.GenericCloudMetric{
				Name:  resources.RedisMemoryUsagePercentageAverage,
				Value: 0.5,
			},
			wantErr: false,
		},
		{
			name: "failure getting metric data",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				opts: opts,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMetricData(context.TODO(), tt.args.metricClient, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMetricData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getMetricData() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getMetrics(t *testing.T) {
	type args struct {
		metricClient gcpiface.MetricApi
		opts         getMetricsOpts
	}
	opts := getMetricsOpts{
		metricsToQuery: []providers.CloudProviderMetricType{
			{
				PrometheusMetricName: resources.RedisMemoryUsagePercentageAverage,
				ProviderMetricName:   "redis.googleapis.com/stats/memory/usage_ratio",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
			{
				PrometheusMetricName: resources.RedisCPUUtilizationAverage,
				ProviderMetricName:   "redis.googleapis.com/stats/cpu_utilization",
				Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
			},
		},
		filterTemplate:         redisMetricFilterTemplate,
		monitoringResourceType: resources.MonitoringResourceTypeRedisInstance,
		projectID:              gcpTestProjectId,
		instanceID:             gcpTestRedisInstanceName,
	}
	tests := []struct {
		name    string
		args    args
		want    []*providers.GenericCloudMetric
		wantErr bool
	}{
		{
			name: "success getting metrics",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
				}),
				opts: opts,
			},
			want: []*providers.GenericCloudMetric{
				{
					Name:  resources.RedisMemoryUsagePercentageAverage,
					Value: 0.5,
				},
				{
					Name:  resources.RedisCPUUtilizationAverage,
					Value: 0.5,
				},
			},
			wantErr: false,
		},
		{
			name: "failure getting metrics",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				opts: opts,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMetrics(context.TODO(), tt.args.metricClient, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMetrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, metric := range tt.want {
				if !metric.IsIncludedInSlice(got) {
					t.Errorf("expected metric %s with value %.1f to be included in slice %v", metric.Name, metric.Value, got)
				}
			}
		})
	}
}

func Test_calculateCpuUtilization(t *testing.T) {
	type args struct {
		metricClient gcpiface.MetricApi
		metric       providers.CloudProviderMetricType
		opts         getMetricsOpts
	}
	metric := providers.CloudProviderMetricType{
		PrometheusMetricName: resources.RedisCPUUtilizationAverage,
		ProviderMetricName:   "redis.googleapis.com/stats/cpu_utilization",
		Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
	}
	opts := getMetricsOpts{
		filterTemplate:         redisMetricFilterTemplate,
		monitoringResourceType: resources.MonitoringResourceTypeRedisInstance,
		projectID:              gcpTestProjectId,
		instanceID:             gcpTestRedisInstanceName,
	}
	tests := []struct {
		name    string
		args    args
		want    *providers.GenericCloudMetric
		wantErr bool
	}{
		{
			name: "success calculating cpu utilization",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									{
										Interval: testInterval,
										Value: &monitoringpb.TypedValue{
											Value: &monitoringpb.TypedValue_DoubleValue{
												DoubleValue: float64(resources.MetricsWatchDuration / time.Second), // simulates 100% cpu usage
											},
										},
									},
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
				}),
				metric: metric,
				opts:   opts,
			},
			want: &providers.GenericCloudMetric{
				Name:  resources.RedisCPUUtilizationAverage,
				Value: 1,
			},
			wantErr: false,
		},
		{
			name: "success calculating cpu utilization when no activity in most recent sample",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									{
										Interval: testInterval,
										Value: &monitoringpb.TypedValue{
											Value: &monitoringpb.TypedValue_DoubleValue{
												DoubleValue: 10, // simulates cpu utilization bump in previous sample
											},
										},
									},
								},
							},
						}, nil
					}
				}),
				metric: metric,
				opts:   opts,
			},
			want: &providers.GenericCloudMetric{
				Name:  resources.RedisCPUUtilizationAverage,
				Value: 0,
			},
			wantErr: false,
		},
		{
			name: "failure calculating cpu utilization - first sample",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				metric: metric,
				opts:   opts,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failure calculating cpu utilization - second sample",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									testPoint,
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				metric: metric,
				opts:   opts,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateCpuUtilization(context.TODO(), tt.args.metricClient, tt.args.metric, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateCpuUtilization() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateCpuUtilization() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateAvailableMemory(t *testing.T) {
	type args struct {
		metricClient   gcpiface.MetricApi
		compoundMetric providers.CloudProviderMetricType
		opts           getMetricsOpts
	}
	metric := providers.CloudProviderMetricType{
		PrometheusMetricName: resources.RedisFreeableMemoryAverage,
		ProviderMetricName:   "redis.googleapis.com/stats/memory/maxmemory-redis.googleapis.com/stats/memory/usage",
		Statistic:            monitoringpb.Aggregation_ALIGN_MEAN.String(),
	}
	opts := getMetricsOpts{
		filterTemplate:         redisMetricFilterTemplate,
		monitoringResourceType: resources.MonitoringResourceTypeRedisInstance,
		projectID:              gcpTestProjectId,
		instanceID:             gcpTestRedisInstanceName,
	}
	tests := []struct {
		name    string
		args    args
		want    *providers.GenericCloudMetric
		wantErr bool
	}{
		{
			name: "success calculating available memory",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									{
										Interval: testInterval,
										Value: &monitoringpb.TypedValue{
											Value: &monitoringpb.TypedValue_DoubleValue{
												DoubleValue: 2147483648, // 2GB max memory
											},
										},
									},
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									{
										Interval: testInterval,
										Value: &monitoringpb.TypedValue{
											Value: &monitoringpb.TypedValue_DoubleValue{
												DoubleValue: 1073741824, // 1GB used memory
											},
										},
									},
								},
							},
						}, nil
					}
				}),
				compoundMetric: metric,
				opts:           opts,
			},
			want: &providers.GenericCloudMetric{
				Name:  resources.RedisFreeableMemoryAverage,
				Value: 1073741824, // 1GB in bytes
			},
			wantErr: false,
		},
		{
			name: "failure calculating available memory - max memory metric",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				compoundMetric: metric,
				opts:           opts,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "failure calculating available memory - used memory metric",
			args: args{
				metricClient: gcpiface.GetMockMetricClient(func(metricClient *gcpiface.MockMetricClient) {
					metricClient.ListTimeSeriesFn = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return []*monitoringpb.TimeSeries{
							{
								Points: []*monitoringpb.Point{
									{
										Interval: testInterval,
										Value: &monitoringpb.TypedValue{
											Value: &monitoringpb.TypedValue_DoubleValue{
												DoubleValue: 2147483648, // 2GB max memory
											},
										},
									},
								},
							},
						}, nil
					}
					metricClient.ListTimeSeriesFnTwo = func(ctx context.Context, request *monitoringpb.ListTimeSeriesRequest, option ...gax.CallOption) ([]*monitoringpb.TimeSeries, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				compoundMetric: metric,
				opts:           opts,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateAvailableMemory(context.TODO(), tt.args.metricClient, tt.args.compoundMetric, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateAvailableMemory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calculateAvailableMemory() got = %v, want %v", got, tt.want)
			}
		})
	}
}
