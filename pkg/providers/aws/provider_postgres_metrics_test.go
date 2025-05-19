package aws

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/rds"
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

	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testMetricName  = "mock_result_id"
	testMetricValue = 1.11111
)

var testMetricLabels = map[string]string{
	resources.LabelClusterIDKey:   "test",
	resources.LabelInstanceIDKey:  "testtesttest",
	resources.LabelNamespaceKey:   "test",
	resources.LabelProductNameKey: "test_product",
	resources.LabelResourceIDKey:  "test",
	resources.LabelStrategyKey:    "aws-rds",
}

func buildProviderMetricType(modifyFn func(*providers.CloudProviderMetricType)) providers.CloudProviderMetricType {
	mock := &providers.CloudProviderMetricType{
		PrometheusMetricName: testMetricName,
		ProviderMetricName:   "test",
		Statistic:            "test",
	}
	if modifyFn != nil {
		modifyFn(mock)
	}
	return *mock
}

type mockCloudWatchClient struct {
	mock.Mock
	cloudwatch.Client
	// Define function fields to mock specific method calls
	getMetricDataFn func(ctx context.Context, input *cloudwatch.GetMetricDataInput, opts ...func(*rds.Options)) (*cloudwatch.GetMetricDataOutput, error)
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
		ctx              context.Context
		cloudWatchClient *cloudwatch.Client
		postgres         *v1alpha1.Postgres
		metricTypes      []providers.CloudProviderMetricType
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
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{
						MetricDataResults: []cloudwatchtypes.MetricDataResult{
							cloudwatchtypes.MetricDataResult{
								Id: aws.String(testMetricName),
								Values: []float64{
									testMetricValue,
								},
							},
						},
					}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				cloudWatchClient: func() *cloudwatch.Client {
					mockCloudWatch := new(mockCloudWatchClient)
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{
						MetricDataResults: []cloudwatchtypes.MetricDataResult{
							cloudwatchtypes.MetricDataResult{
								Id: aws.String(testMetricName),
								Values: []float64{
									testMetricValue,
								},
								StatusCode: cloudwatchtypes.StatusCodeInternalError,
							},
						},
					}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				CredentialManager: &CredentialManagerMock{},
				ConfigManager:     &ConfigManagerMock{},
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				cloudWatchClient: func() *cloudwatch.Client {
					mockCloudWatch := new(mockCloudWatchClient)
					mockCloudWatch.On("GetMetricData", mock.Anything, mock.Anything, mock.Anything).Return(&cloudwatch.GetMetricDataOutput{}, nil)
					return (*cloudwatch.Client)(unsafe.Pointer(mockCloudWatch))
				}(),
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
			got, err := p.scrapeRDSCloudWatchMetricData(tt.args.ctx, *tt.args.cloudWatchClient, tt.args.postgres, tt.args.metricTypes)
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
