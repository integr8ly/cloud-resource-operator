package aws

import (
	"context"
	"errors"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/stretchr/testify/mock"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
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

func buildReplicationGroupReadyCacheClusterId() []elasticachetypes.ReplicationGroup {

	return []elasticachetypes.ReplicationGroup{
		{
			ReplicationGroupId:     aws.String("testtesttest"),
			Status:                 aws.String("available"),
			CacheNodeType:          aws.String("test"),
			SnapshotRetentionLimit: aws.Int32(20),
			MemberClusters:         []string{testcacheClusterId1, testcacheClusterId2},
			NodeGroups: []elasticachetypes.NodeGroup{
				{
					NodeGroupId:      aws.String("primary-node"),
					NodeGroupMembers: nil,
					PrimaryEndpoint: &elasticachetypes.Endpoint{
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
		ctx               context.Context
		cloudWatchClient  *cloudwatch.Client
		redis             *v1alpha1.Redis
		elastiCacheClient *elasticache.Client
		metricTypes       []providers.CloudProviderMetricType
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
				cloudWatchClient: func() *cloudwatch.Client {
					mockCloudWatch := new(mockCloudWatchClient)
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{
						MetricDataResults: []cloudwatchtypes.MetricDataResult{
							{
								Id:     aws.String(testMetricName),
								Values: []float64{testMetricValue},
							},
						},
					}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
				elastiCacheClient: func() *elasticache.Client {
					mockElasticache := new(mockElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: buildReplicationGroupReadyCacheClusterId(),
					}, nil)
					return (*elasticache.Client)(unsafe.Pointer(mockElasticache))
				}(),
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
				cloudWatchClient: func() *cloudwatch.Client {
					mockCloudWatch := new(mockCloudWatchClient)
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{
						MetricDataResults: []cloudwatchtypes.MetricDataResult{
							{
								Id:         aws.String(testMetricName),
								Values:     []float64{testMetricValue},
								StatusCode: cloudwatchtypes.StatusCodeInternalError,
							},
						},
					}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
				elastiCacheClient: func() *elasticache.Client {
					mockElasticache := new(mockElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{
						ReplicationGroups: buildReplicationGroupReadyCacheClusterId(),
					}, nil)
					return (*elasticache.Client)(unsafe.Pointer(mockElasticache))
				}(),
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
				cloudWatchClient: func() *cloudwatch.Client {
					mockCloudWatch := new(mockCloudWatchClient)
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
				elastiCacheClient: func() *elasticache.Client {
					mockElasticache := new(mockElasticacheClient)
					mockElasticache.On("DescribeReplicationGroups", mock.Anything, mock.Anything).Return(&elasticache.DescribeReplicationGroupsOutput{}, nil)
					return (*elasticache.Client)(unsafe.Pointer(mockElasticache))
				}(),
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
			got, err := r.scrapeRedisCloudWatchMetricData(tt.args.ctx, *tt.args.cloudWatchClient, tt.args.redis, *tt.args.elastiCacheClient, tt.args.metricTypes)
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
