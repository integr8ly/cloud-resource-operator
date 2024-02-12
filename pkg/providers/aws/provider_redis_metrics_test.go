package aws

import (
	"context"
	"errors"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"os"
	"reflect"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elasticache/elasticacheiface"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/moq/moq_aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testRedisMetricName  = "mock_result_id"
	testRedisMetricValue = 1.11111
)

var (
	testcacheClusterId1 = "test-001"
	testcacheClusterId2 = "test-002"
)

func buildReplicationGroupReadyCacheClusterId() []*elasticache.ReplicationGroup {

	return []*elasticache.ReplicationGroup{
		{
			ReplicationGroupId:     aws.String("testtesttest"),
			Status:                 aws.String("available"),
			CacheNodeType:          aws.String("test"),
			SnapshotRetentionLimit: aws.Int64(20),
			MemberClusters:         []*string{&testcacheClusterId1, &testcacheClusterId2},
			NodeGroups: []*elasticache.NodeGroup{
				{
					NodeGroupId:      aws.String("primary-node"),
					NodeGroupMembers: nil,
					PrimaryEndpoint: &elasticache.Endpoint{
						Address: testAddress,
						Port:    testPort,
					},
					Status: aws.String("available"),
				},
			},
		},
	}
}

func moqRedisMetricLabels(instanceID string) (labels map[string]string) {
	return map[string]string{
		resources.LabelClusterIDKey:   "test",
		resources.LabelInstanceIDKey:  instanceID,
		resources.LabelNamespaceKey:   "test",
		resources.LabelProductNameKey: "",
		resources.LabelResourceIDKey:  "test",
		resources.LabelStrategyKey:    "aws-elasticache",
	}
}
func TestRedisMetricsProvider_scrapeRedisCloudWatchMetricData(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx            context.Context
		cloudWatchApi  cloudwatchiface.CloudWatchAPI
		redis          *v1alpha1.Redis
		elastiCacheApi elasticacheiface.ElastiCacheAPI
		metricTypes    []providers.CloudProviderMetricType
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*providers.GenericCloudMetric
		wantErr bool
	}{
		{
			name: "test successful scrape of cloud watch metrics",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				cloudWatchApi: moq_aws.BuildMockCloudWatchClient(func(watchClient *moq_aws.MockCloudWatchClient) {
					watchClient.GetMetricDataFn = func(input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
						return &cloudwatch.GetMetricDataOutput{
							MetricDataResults: []*cloudwatch.MetricDataResult{
								moq_aws.BuildMockMetricDataResult(func(result *cloudwatch.MetricDataResult) {
									result.Id = aws.String(testMetricName)
									result.Values = []*float64{
										aws.Float64(testMetricValue),
									}
								}),
							},
						}, nil
					}
				}),
				elastiCacheApi: moq_aws.BuildMockElastiCacheClient(func(watchClient *moq_aws.MockElastiCacheClient) {
					watchClient.DescribeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: buildReplicationGroupReadyCacheClusterId(),
						}, nil
					}
				}),
				redis: buildTestRedisCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			want: []*providers.GenericCloudMetric{
				{
					Name:   testRedisMetricName,
					Value:  testRedisMetricValue,
					Labels: moqRedisMetricLabels(testcacheClusterId1),
				},
				{
					Name:   testRedisMetricName,
					Value:  testRedisMetricValue,
					Labels: moqRedisMetricLabels(testcacheClusterId2),
				},
			},
			wantErr: false,
		},
		{
			name: "test successful scrape of cloud watch metrics, with 1 not complete metric",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				cloudWatchApi: moq_aws.BuildMockCloudWatchClient(func(watchClient *moq_aws.MockCloudWatchClient) {
					watchClient.GetMetricDataFn = func(input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
						return &cloudwatch.GetMetricDataOutput{
							MetricDataResults: []*cloudwatch.MetricDataResult{
								moq_aws.BuildMockMetricDataResult(func(result *cloudwatch.MetricDataResult) {
									result.Id = aws.String(testMetricName)
									result.Values = []*float64{
										aws.Float64(testMetricValue),
									}
								}),
								moq_aws.BuildMockMetricDataResult(func(result *cloudwatch.MetricDataResult) {
									result.StatusCode = aws.String(cloudwatch.StatusCodeInternalError)
								}),
							},
						}, nil
					}
				}),
				elastiCacheApi: moq_aws.BuildMockElastiCacheClient(func(watchClient *moq_aws.MockElastiCacheClient) {
					watchClient.DescribeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{
							ReplicationGroups: buildReplicationGroupReadyCacheClusterId(),
						}, nil
					}
				}),
				redis: buildTestRedisCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			want: []*providers.GenericCloudMetric{
				{
					Name:   testRedisMetricName,
					Value:  testRedisMetricValue,
					Labels: moqRedisMetricLabels(testcacheClusterId1),
				},
				{
					Name:   testRedisMetricName,
					Value:  testRedisMetricValue,
					Labels: moqRedisMetricLabels(testcacheClusterId2),
				},
			},
			wantErr: false,
		},
		{
			name: "test no metrics have been returned from cloudwatch scrape",
			fields: fields{
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				cloudWatchApi: moq_aws.BuildMockCloudWatchClient(func(watchClient *moq_aws.MockCloudWatchClient) {
					watchClient.GetMetricDataFn = func(input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
						return &cloudwatch.GetMetricDataOutput{}, nil
					}
				}),
				elastiCacheApi: moq_aws.BuildMockElastiCacheClient(func(watchClient *moq_aws.MockElastiCacheClient) {
					watchClient.DescribeReplicationGroupsFn = func(input *elasticache.DescribeReplicationGroupsInput) (*elasticache.DescribeReplicationGroupsOutput, error) {
						return &elasticache.DescribeReplicationGroupsOutput{}, nil
					}
				}),
				redis: buildTestRedisCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisMetricsProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := r.scrapeRedisCloudWatchMetricData(tt.args.ctx, tt.args.cloudWatchApi, tt.args.redis, tt.args.elastiCacheApi, tt.args.metricTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("scrapeRedisCloudWatchMetricData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scrapeRedisCloudWatchMetricData() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAWSRedisMetricsProvider(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	if k8sutil.IsRunModeLocal() {
		_ = os.Setenv("WATCH_NAMESPACE", "test")
	}
	type args struct {
		client func() client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name    string
		args    args
		want    *RedisMetricsProvider
		wantErr bool
	}{
		{
			name: "successfully create new redis metrics provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: false,
		},
		{
			name: "fail to create new redis metrics provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return errors.New("generic error")
					}
					return mockClient
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAWSRedisMetricsProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSRedisMetricsProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSRedisMetricsProvider() got = %v, want non-nil result", got)
			}
		})
	}
}
