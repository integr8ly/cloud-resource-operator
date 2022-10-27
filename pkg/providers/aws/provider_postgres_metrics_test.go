package aws

import (
	"context"
	"errors"
	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"os"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/moq/moq_aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testMetricName  = "mock_result_id"
	testMetricValue = 1.11111
)

var testMetricLabels = map[string]string{
	"clusterID":   "test",
	"instanceID":  "testtesttest",
	"namespace":   "test",
	"productName": "test_product",
	"resourceID":  "test",
	"strategy":    "aws-rds",
}

func buildProviderMetricType(modifyFn func(*providers.CloudProviderMetricType)) providers.CloudProviderMetricType {
	mock := &providers.CloudProviderMetricType{
		PromethuesMetricName: testMetricName,
		ProviderMetricName:   "test",
		Statistic:            "test",
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return *mock
}

func TestPostgresMetricsProvider_scrapeRDSCloudWatchMetricData(t *testing.T) {
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
		ctx           context.Context
		cloudWatchApi cloudwatchiface.CloudWatchAPI
		postgres      *v1alpha1.Postgres
		metricTypes   []providers.CloudProviderMetricType
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
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
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
				postgres: buildTestPostgresCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			want: []*providers.GenericCloudMetric{
				{
					Name:   testMetricName,
					Value:  testMetricValue,
					Labels: testMetricLabels,
				},
			},
			wantErr: false,
		},
		{
			name: "test successful scrape of cloud watch metrics, with 1 not complete metric",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
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
				postgres: buildTestPostgresCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			want: []*providers.GenericCloudMetric{
				{
					Name:   testMetricName,
					Value:  testMetricValue,
					Labels: testMetricLabels,
				},
			},
			wantErr: false,
		},
		{
			name: "test no metrics have been returned from cloudwatch scrape",
			fields: fields{
				Client:            fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
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
				postgres: buildTestPostgresCR(),
				metricTypes: []providers.CloudProviderMetricType{
					buildProviderMetricType(func(metricType *providers.CloudProviderMetricType) {}),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresMetricsProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.scrapeRDSCloudWatchMetricData(tt.args.ctx, tt.args.cloudWatchApi, tt.args.postgres, tt.args.metricTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("scrapeRDSCloudWatchMetricData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scrapeRDSCloudWatchMetricData() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAWSPostgresMetricsProvider(t *testing.T) {
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
		want    *PostgresMetricsProvider
		wantErr bool
	}{
		{
			name: "successfully create new postgres metrics provider",
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
			name: "fail to create new postgres metrics provider",
			args: args{
				client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
			got, err := NewAWSPostgresMetricsProvider(tt.args.client(), tt.args.logger)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewAWSPostgresMetricsProvider(), got = %v, want non-nil error", err)
				}
				return
			}
			if got == nil {
				t.Errorf("NewAWSPostgresMetricsProvider() got = %v, want non-nil result", got)
			}
		})
	}
}
