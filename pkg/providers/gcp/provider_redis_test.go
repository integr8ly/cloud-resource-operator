package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"google.golang.org/api/servicenetworking/v1"
	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/genproto/googleapis/type/timeofday"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	redis "cloud.google.com/go/redis/apiv1"
	"cloud.google.com/go/redis/apiv1/redispb"
	"github.com/googleapis/gax-go/v2"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	utils "k8s.io/utils/pointer"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const gcpTestRedisInstanceName = "projects/" + gcpTestProjectId + "/locations/" + gcpTestRegion + "/instances/" + testName

func buildTestComputeAddress(argsMap map[string]string) *computepb.Address {
	address := &computepb.Address{
		Name:    utils.String(gcpTestIpRangeName),
		Network: utils.String(fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", gcpTestProjectId, gcpTestNetworkName)),
	}
	if argsMap != nil {
		if argsMap["status"] != "" {
			address.Status = utils.String(argsMap["status"])
		}
	}
	return address
}

func TestNewGCPRedisProvider(t *testing.T) {
	type args struct {
		client client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "placeholder test",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGCPRedisProvider(tt.args.client, tt.args.logger); got == nil {
				t.Errorf("NewGCPRedisProvider() got = %v, want non-nil result", got)
			}
		})
	}
}

func TestRedisProvider_deleteRedisInstance(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx            context.Context
		networkManager NetworkManager
		redisClient    gcpiface.RedisAPI
		strategyConfig *StrategyConfig
		r              *v1alpha1.Redis
		isLastResource bool
	}
	scheme := runtime.NewScheme()
	_ = configv1.Install(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	instanceID := fmt.Sprintf("projects/%s/locations/%s/instances/%s", gcpTestProjectId, gcpTestRegion, testName)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "success triggering deletion for an existing redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
				),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							Name:  gcpTestRedisInstanceName,
							State: redispb.Instance_READY,
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("delete detected, gcp redis instance %s deletion started", instanceID)),
			wantErr: false,
		},
		{
			name: "success reconciling when an existing redis instance is already in progress of deletion",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
				),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							Name:  gcpTestRedisInstanceName,
							State: redispb.Instance_DELETING,
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("deletion in progress for gcp redis instance %s", instanceID)),
			wantErr: false,
		},
		{
			name: "success reconciling when the redis instance deletion and cleanup have completed",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
					&v1alpha1.Redis{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: testName,
							},
							Finalizers: []string{
								DefaultFinalizer,
							},
							Name:      testName,
							Namespace: testNs,
						},
						Spec: types.ResourceTypeSpec{
							Tier: "development",
						},
					},
				),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:            testName,
						Namespace:       testNs,
						ResourceVersion: "999",
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("successfully deleted gcp redis instance %s", instanceID)),
			wantErr: false,
		},
		{
			name:   "fail to build delete redis instance request",
			fields: fields{},
			args: args{
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					DeleteStrategy: nil,
				},
			},
			want:    types.StatusMessage("failed to build delete gcp redis instance request"),
			wantErr: true,
		},
		{
			name: "fail to delete redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
				),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							Name:  gcpTestRedisInstanceName,
							State: redispb.Instance_READY,
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to delete gcp redis instance %s", instanceID)),
			wantErr: true,
		},
		{
			name: "fail to retrieve redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
				),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, fmt.Errorf("generic error")
					}

				}),
				strategyConfig: buildTestStrategyConfig(),
			},
			want:    types.StatusMessage("failed to fetch gcp redis instance " + gcpTestRedisInstanceName),
			wantErr: true,
		},
		{
			name: "fail to update redis instance as part of finalizer reconcile",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to update instance %s as part of finalizer reconcile", testName)),
			wantErr: true,
		},
		{
			name: "fail to delete cluster network peering",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: true,
			},
			want:    "failed to delete cluster network peering",
			wantErr: true,
		},
		{
			name: "fail to delete network service",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: true,
			},
			want:    "failed to delete network service",
			wantErr: true,
		},
		{
			name: "fail to delete network ip range",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return fmt.Errorf("generic error")
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: true,
			},
			want:    "failed to delete network ip range",
			wantErr: true,
		},
		{
			name: "successfully reconcile when components deletion is in progress",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					ComponentsExistFunc: func(contextMoqParam context.Context) (bool, error) {
						return true, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: true,
			},
			want:    "network component deletion in progress",
			wantErr: false,
		},
		{
			name: "fail to check if components exist",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme,
						buildTestGcpInfrastructure(nil),
						buildTestGcpStrategyConfigMap(nil),
					)
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
				networkManager: &NetworkManagerMock{
					DeleteNetworkPeeringFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkServiceFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					DeleteNetworkIpRangeFunc: func(contextMoqParam context.Context) error {
						return nil
					},
					ComponentsExistFunc: func(contextMoqParam context.Context) (bool, error) {
						return false, fmt.Errorf("generic error")
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, resources.NewMockAPIError(grpcCodes.NotFound)
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: true,
			},
			want:    "failed to check if components exist",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGCPRedisProvider(tt.fields.Client, logrus.NewEntry(logrus.StandardLogger()))
			statusMessage, err := p.deleteRedisInstance(context.TODO(), tt.args.networkManager, tt.args.redisClient, tt.args.strategyConfig, tt.args.r, tt.args.isLastResource)
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteRedisInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if statusMessage != tt.want {
				t.Errorf("deleteRedisInstance() statusMessage = %v, want %v", statusMessage, tt.want)
			}
		})
	}
}

func TestRedisProvider_GetName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "success getting redis provider name",
			want: redisProviderName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := RedisProvider{}
			if got := rp.GetName(); got != tt.want {
				t.Errorf("GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_SupportsStrategy(t *testing.T) {
	type args struct {
		deploymentStrategy string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "redis provider supports strategy",
			args: args{
				deploymentStrategy: providers.GCPDeploymentStrategy,
			},
			want: true,
		},
		{
			name: "redis provider does not support strategy",
			args: args{
				deploymentStrategy: providers.AWSDeploymentStrategy,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := RedisProvider{}
			if got := rp.SupportsStrategy(tt.args.deploymentStrategy); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		r *v1alpha1.Redis
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "get redis default reconcile time",
			args: args{
				r: &v1alpha1.Redis{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
		{
			name: "get redis non-default reconcile time",
			args: args{
				r: &v1alpha1.Redis{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseInProgress,
					},
				},
			},
			want: time.Second * 60,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := RedisProvider{}
			if got := rp.GetReconcileTime(tt.args.r); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_buildDeleteInstanceRequest(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		r              *v1alpha1.Redis
		strategyConfig *StrategyConfig
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *redispb.DeleteInstanceRequest
		wantErr bool
	}{
		{
			name:   "success building redis delete instance request from strategy config",
			fields: fields{},
			args: args{
				r: nil,
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					DeleteStrategy: json.RawMessage(fmt.Sprintf(`{"name":"projects/%s/locations/%s/instances/%s"}`, gcpTestProjectId, gcpTestRegion, testName)),
				},
			},
			want: &redispb.DeleteInstanceRequest{
				Name: gcpTestRedisInstanceName,
			},
			wantErr: false,
		},
		{
			name:   "success building redis delete instance request from strategy config and cr annotations",
			fields: fields{},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					DeleteStrategy: json.RawMessage(`{}`),
				},
			},
			want: &redispb.DeleteInstanceRequest{
				Name: gcpTestRedisInstanceName,
			},
			wantErr: false,
		},
		{
			name:   "fail to unmarshal gcp redis delete strategy",
			fields: fields{},
			args: args{
				r: nil,
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					DeleteStrategy: nil,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "fail to find redis instance name from annotations",
			fields: fields{},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					DeleteStrategy: json.RawMessage(`{}`),
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.buildDeleteInstanceRequest(tt.args.r, tt.args.strategyConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildDeleteInstanceRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildDeleteInstanceRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_buildCreateInstanceRequest(t *testing.T) {
	scheme, _ := buildTestScheme()
	parent := fmt.Sprintf(redisParentFormat, gcpTestProjectId, gcpTestRegion)
	instanceID := "gcptestclustertestNstestName"
	redisInstance := &redispb.Instance{
		Name:              fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, instanceID),
		Tier:              redispb.Instance_STANDARD_HA,
		ReadReplicasMode:  redispb.Instance_READ_REPLICAS_DISABLED,
		MemorySizeGb:      redisMemorySizeGB,
		AuthorizedNetwork: fmt.Sprintf("projects/%s/global/networks/%s", gcpTestProjectId, gcpTestNetworkName),
		ConnectMode:       redispb.Instance_PRIVATE_SERVICE_ACCESS,
		ReservedIpRange:   gcpTestIpRangeName,
		RedisVersion:      redisVersion,
		Labels: map[string]string{
			"integreatly-org_clusterid":     gcpTestClusterName,
			"integreatly-org_resource-name": "testname",
			"integreatly-org_resource-type": "",
			"red-hat-managed":               "true",
		},
	}
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx            context.Context
		r              *v1alpha1.Redis
		strategyConfig *StrategyConfig
		address        *computepb.Address
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *redispb.CreateInstanceRequest
		wantErr bool
	}{
		{
			name: "success building redis create instance request from strategy config",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:    gcpTestRegion,
					ProjectID: gcpTestProjectId,
					CreateStrategy: func() json.RawMessage {
						instance := `{"name":"","tier":0,"read_replicas_mode":0,"memory_size_gb":0,"authorized_network":"","connect_mode":0,"reserved_ip_range":"","redis_version":""}`
						return json.RawMessage(fmt.Sprintf(`{"parent":"%s","instance_id":"%s","instance":%s}`, parent, instanceID, instance))
					}(),
				},
				address: buildTestComputeAddress(nil),
			},
			want: &redispb.CreateInstanceRequest{
				Parent:     parent,
				InstanceId: instanceID,
				Instance:   redisInstance,
			},
			wantErr: false,
		},
		{
			name: "success building redis create instance request with default values",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				address: buildTestComputeAddress(nil),
			},
			want: &redispb.CreateInstanceRequest{
				Parent:     parent,
				InstanceId: instanceID,
				Instance:   redisInstance,
			},
			wantErr: false,
		},
		{
			name: "success building redis create instance request with custom size",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Size: "7",
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				address: buildTestComputeAddress(nil),
			},
			want: &redispb.CreateInstanceRequest{
				Parent:     parent,
				InstanceId: instanceID,
				Instance: func() *redispb.Instance {
					inst := *redisInstance
					inst.MemorySizeGb = 7
					return &inst
				}(),
			},
			wantErr: false,
		},
		{
			name: "fail to parse redis spec size",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Size: "invalid",
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				address: buildTestComputeAddress(nil),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "fail to unmarshal gcp redis create strategy",
			fields: fields{},
			args: args{
				r: nil,
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: nil,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to build redis instance id from object",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme)
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				address: buildTestComputeAddress(nil),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := p.buildCreateInstanceRequest(context.TODO(), tt.args.r, tt.args.strategyConfig, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCreateInstanceRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildCreateInstanceRequest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_createRedisInstance(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = configv1.Install(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	redisCR := &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNs,
		},
	}
	instanceID := fmt.Sprintf("projects/%s/locations/%s/instances/%s", gcpTestProjectId, gcpTestRegion, "gcptestclustertestNstestName")
	instanceID2 := fmt.Sprintf("projects/%s/locations/%s/instances/%s", gcpTestProjectId, gcpTestRegion, "gcptestcluster")
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx            context.Context
		networkManager NetworkManager
		redisClient    gcpiface.RedisAPI
		strategyConfig *StrategyConfig
		r              *v1alpha1.Redis
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		redisCluster  *providers.RedisCluster
		statusMessage types.StatusMessage
		wantErr       bool
	}{
		{
			name: "success creating a gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil), redisCR),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State: redispb.Instance_READY,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster: &providers.RedisCluster{
				DeploymentDetails: &providers.RedisDeploymentDetails{},
			},
			statusMessage: types.StatusMessage("successfully reconciled gcp redis instance " + instanceID),
			wantErr:       false,
		},
		{
			name:   "fail to reconcile network provider config",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return nil, fmt.Errorf("generic error")
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to reconcile network provider config",
			wantErr:       true,
		},
		{
			name:   "fail to create network service",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return nil, types.StatusNetworkCreateError, fmt.Errorf("generic error")
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: types.StatusNetworkCreateError,
			wantErr:       true,
		},
		{
			name:   "fail to create network service",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return nil, types.StatusNetworkCreateError, fmt.Errorf("generic error")
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: types.StatusNetworkCreateError,
			wantErr:       true,
		},
		{
			name:   "fail to build create redis instance request",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				strategyConfig: &StrategyConfig{
					CreateStrategy: nil,
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to build gcp create redis instance request",
			wantErr:       true,
		},
		{
			name: "fail to fetch redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage(fmt.Sprintf("failed to fetch gcp redis instance %s", instanceID2)),
			wantErr:       true,
		},
		{
			name: "fail to create redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, nil
					}
					redisClient.CreateInstanceFn = func(ctx context.Context, request *redispb.CreateInstanceRequest, option ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage(fmt.Sprintf("failed to create gcp redis instance %s", instanceID2)),
			wantErr:       true,
		},
		{
			name: "fail to add annotation to redis cr",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil))
					mockClient.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, nil
					}
					redisClient.CreateInstanceFn = func(ctx context.Context, request *redispb.CreateInstanceRequest, option ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
						return nil, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestProjectId,
					ProjectID:      gcpTestRegion,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to add annotation to redis cr",
			wantErr:       true,
		},
		{
			name: "fail to add annotation to redis cr when instance already exists",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil))
					mockClient.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State: redispb.Instance_CREATING,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestProjectId,
					ProjectID:      gcpTestRegion,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to add annotation to redis cr",
			wantErr:       true,
		},
		{
			name: "start creation of gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil), redisCR),
			},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, nil
					}
					redisClient.CreateInstanceFn = func(ctx context.Context, request *redispb.CreateInstanceRequest, option ...gax.CallOption) (*redis.CreateInstanceOperation, error) {
						return nil, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: redisCR,
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage(fmt.Sprintf("started creation of gcp redis instance %s", instanceID)),
			wantErr:       false,
		},
		{
			name: "redis instance creation in progress",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State: redispb.Instance_CREATING,
						}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage(fmt.Sprintf("gcp redis instance %s is not ready yet, current state is %s", instanceID, redispb.Instance_CREATING.String())),
			wantErr:       false,
		},
		{
			name: "success updating a gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State:        redispb.Instance_READY,
							RedisVersion: redisVersion,
						}, nil
					}
					redisClient.UpdateInstanceFn = func(ctx context.Context, request *redispb.UpdateInstanceRequest, option ...gax.CallOption) (*redis.UpdateInstanceOperation, error) {
						return &redis.UpdateInstanceOperation{}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"memory_size_gb": 3}}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster: &providers.RedisCluster{
				DeploymentDetails: &providers.RedisDeploymentDetails{},
			},
			statusMessage: types.StatusMessage("successfully reconciled gcp redis instance " + instanceID),
			wantErr:       false,
		},
		{
			name: "success upgrading a gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State:        redispb.Instance_READY,
							RedisVersion: "REDIS_5_0",
							MemorySizeGb: redisMemorySizeGB,
							Labels: map[string]string{
								"integreatly.org/clusterID":     gcpTestClusterName,
								"integreatly.org/resource-type": "",
								"integreatly.org/resource-name": testName,
								resources.TagManagedKey:         "true",
							},
						}, nil
					}
					redisClient.UpgradeInstanceFn = func(ctx context.Context, request *redispb.UpgradeInstanceRequest, option ...gax.CallOption) (*redis.UpgradeInstanceOperation, error) {
						return &redis.UpgradeInstanceOperation{}, nil
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"redis_version":"REDIS_6_X"}}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster: &providers.RedisCluster{
				DeploymentDetails: &providers.RedisDeploymentDetails{},
			},
			statusMessage: types.StatusMessage("successfully reconciled gcp redis instance " + instanceID),
			wantErr:       false,
		},
		{
			name: "failure updating a gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State:        redispb.Instance_READY,
							RedisVersion: redisVersion,
						}, nil
					}
					redisClient.UpdateInstanceFn = func(ctx context.Context, request *redispb.UpdateInstanceRequest, option ...gax.CallOption) (*redis.UpdateInstanceOperation, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"memory_size_gb": 3}}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage("failed to update gcp redis instance " + instanceID),
			wantErr:       true,
		},
		{
			name: "failure upgrading a gcp redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, types.StatusMessage, error) {
						return buildTestComputeAddress(map[string]string{"status": computepb.Address_RESERVED.String()}), "", nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, types.StatusMessage, error) {
						return &servicenetworking.Connection{}, "", nil
					},
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return &redispb.Instance{
							State:        redispb.Instance_READY,
							RedisVersion: "REDIS_5_0",
							MemorySizeGb: redisMemorySizeGB,
							Labels: map[string]string{
								"integreatly.org/clusterID":     gcpTestClusterName,
								"integreatly.org/resource-type": "",
								"integreatly.org/resource-name": testName,
								resources.TagManagedKey:         "true",
							},
						}, nil
					}
					redisClient.UpgradeInstanceFn = func(ctx context.Context, request *redispb.UpgradeInstanceRequest, option ...gax.CallOption) (*redis.UpgradeInstanceOperation, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: &StrategyConfig{
					Region:         gcpTestRegion,
					ProjectID:      gcpTestProjectId,
					CreateStrategy: json.RawMessage(`{"instance":{"redis_version":"REDIS_6_X"}}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster:  nil,
			statusMessage: types.StatusMessage("failed to upgrade gcp redis instance " + instanceID),
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
				TCPPinger:         resources.BuildMockConnectionTester(),
			}
			redisCluster, statusMessage, err := p.createRedisInstance(context.TODO(), tt.args.networkManager, tt.args.redisClient, tt.args.strategyConfig, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRedisInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(redisCluster, tt.redisCluster) {
				t.Errorf("createRedisInstance() redisCluster = %v, want %v", redisCluster, tt.redisCluster)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("createRedisInstance() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestRedisProvider_buildUpdateInstanceRequest(t *testing.T) {
	type args struct {
		instanceConfig *redispb.Instance
		instance       *redispb.Instance
	}
	tests := []struct {
		name string
		args args
		want *redispb.UpdateInstanceRequest
	}{
		{
			name: "success building gcp redis update instance request when update is found",
			args: args{
				instanceConfig: &redispb.Instance{
					Labels: map[string]string{
						"testKey": "testValue",
					},
					RedisConfigs: map[string]string{
						"testKey": "testValue",
					},
					MemorySizeGb: 1,

					MaintenancePolicy: &redispb.MaintenancePolicy{
						WeeklyMaintenanceWindow: []*redispb.WeeklyMaintenanceWindow{
							{
								Day: dayofweek.DayOfWeek_TUESDAY,
								StartTime: &timeofday.TimeOfDay{
									Hours: 9,
								},
								Duration: durationpb.New(time.Hour),
							},
						},
					},
				},
				instance: &redispb.Instance{},
			},
			want: &redispb.UpdateInstanceRequest{
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"memory_size_gb", "labels", "redis_configs", "maintenance_policy"},
				},
				Instance: &redispb.Instance{
					Labels: map[string]string{
						"testKey": "testValue",
					},
					RedisConfigs: map[string]string{
						"testKey": "testValue",
					},
					MemorySizeGb: 1,
					MaintenancePolicy: &redispb.MaintenancePolicy{
						WeeklyMaintenanceWindow: []*redispb.WeeklyMaintenanceWindow{
							{
								Day: dayofweek.DayOfWeek_TUESDAY,
								StartTime: &timeofday.TimeOfDay{
									Hours: 9,
								},
								Duration: durationpb.New(time.Hour),
							},
						},
					},
				},
			},
		},
		{
			name: "success skipping build of gcp redis update instance request when update is not found",
			args: args{
				instanceConfig: &redispb.Instance{},
				instance:       &redispb.Instance{},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{}
			if got := p.buildUpdateInstanceRequest(tt.args.instanceConfig, tt.args.instance); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildUpdateInstanceRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_buildUpgradeInstanceRequest(t *testing.T) {
	type args struct {
		instanceConfig *redispb.Instance
		instance       *redispb.Instance
	}
	tests := []struct {
		name string
		args args
		want *redispb.UpgradeInstanceRequest
	}{
		{
			name: "success building gcp redis upgrade instance request when upgrade is found",
			args: args{
				instanceConfig: &redispb.Instance{
					Name:         testName,
					RedisVersion: redisVersion,
				},
				instance: &redispb.Instance{
					Name:         testName,
					RedisVersion: "REDIS_5_0",
				},
			},
			want: &redispb.UpgradeInstanceRequest{
				Name:         testName,
				RedisVersion: redisVersion,
			},
		},
		{
			name: "success skipping build of gcp redis upgrade instance request when upgrade is not found",
			args: args{
				instanceConfig: &redispb.Instance{
					Name:         testName,
					RedisVersion: redisVersion,
				},
				instance: &redispb.Instance{
					Name:         testName,
					RedisVersion: redisVersion,
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{}
			if got := p.buildUpgradeInstanceRequest(tt.args.instanceConfig, tt.args.instance); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildUpgradeInstanceRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isMaintenancePolicyOutdated(t *testing.T) {
	type args struct {
		a *redispb.MaintenancePolicy
		b *redispb.MaintenancePolicy
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "instance policy is not outdated - both maintenance policies are undefined",
			args: args{
				a: nil,
				b: nil,
			},
			want: false,
		},
		{
			name: "instance policy is outdated - one maintenance policy is undefined",
			args: args{
				a: nil,
				b: &redispb.MaintenancePolicy{},
			},
			want: true,
		},
		{
			name: "instance policy is outdated - one maintenance window differs",
			args: args{
				a: &redispb.MaintenancePolicy{
					WeeklyMaintenanceWindow: []*redispb.WeeklyMaintenanceWindow{{}},
				},
				b: &redispb.MaintenancePolicy{
					WeeklyMaintenanceWindow: []*redispb.WeeklyMaintenanceWindow{
						{
							Day: dayofweek.DayOfWeek_TUESDAY,
							StartTime: &timeofday.TimeOfDay{
								Hours: 9,
							},
							Duration: durationpb.New(time.Hour),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "instance policy is not outdated - deep equality",
			args: args{
				a: &redispb.MaintenancePolicy{},
				b: &redispb.MaintenancePolicy{},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMaintenancePolicyOutdated(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("isMaintenancePolicyOutdated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_exposeRedisInstanceMetrics(t *testing.T) {
	type fields struct {
		Client    client.Client
		TCPPinger resources.ConnectionTester
	}
	type args struct {
		r        *v1alpha1.Redis
		instance *redispb.Instance
	}
	scheme := runtime.NewScheme()
	_ = configv1.AddToScheme(scheme)
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "success exposing gcp redis instance metrics",
			fields: fields{
				Client:    moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				TCPPinger: resources.BuildMockConnectionTester(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
						Labels: map[string]string{
							"productName": testName,
						},
					},
				},
				instance: &redispb.Instance{
					State: redispb.Instance_READY,
					Host:  "0.0.0.0",
					Port:  5432,
				},
			},
		},
		{
			name:   "exit early if the gcp redis instance is nil",
			fields: fields{},
			args:   args{},
		},
		{
			name: "failure getting cluster id while exposing metrics for gcp redis instance",
			fields: fields{
				Client: func() client.Client {
					mockClient := moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil))
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mockClient
				}(),
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
						Labels: map[string]string{
							"productName": testName,
						},
					},
				},
				instance: &redispb.Instance{
					State: redispb.Instance_READY,
					Host:  "0.0.0.0",
					Port:  5432,
				},
			},
		},
		{
			name: "success exposing gcp redis instance metrics when ping instance fails",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				TCPPinger: &resources.ConnectionTesterMock{
					TCPConnectionFunc: func(host string, port int) bool {
						return false
					},
				},
			},
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						Name:      testName,
						Namespace: testNs,
						Labels: map[string]string{
							"productName": testName,
						},
					},
				},
				instance: &redispb.Instance{
					State: redispb.Instance_READY,
					Host:  "0.0.0.0",
					Port:  5432,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:    tt.fields.Client,
				Logger:    logrus.NewEntry(logrus.StandardLogger()),
				TCPPinger: tt.fields.TCPPinger,
			}
			p.exposeRedisInstanceMetrics(context.TODO(), tt.args.r, tt.args.instance)
		})
	}
}

func TestRedisProvider_setRedisDeletionTimestampMetric(t *testing.T) {
	type fields struct {
		Client client.Client
	}
	type args struct {
		ctx context.Context
		r   *v1alpha1.Redis
	}
	scheme := runtime.NewScheme()
	_ = configv1.AddToScheme(scheme)
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "success setting redis deletion timestamp metric",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				ctx: context.TODO(),
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
						Name:      testName,
						Namespace: testNs,
					},
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseDeleteInProgress,
					},
				},
			},
		},
		{
			name: "failure fetching resource identifier annotation",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
			},
			args: args{
				ctx: context.TODO(),
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
						Name:      testName,
						Namespace: testNs,
					},
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseDeleteInProgress,
					},
				},
			},
		},
		{
			name: "failure fetching cluster id",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			args: args{
				ctx: context.TODO(),
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: testName,
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
						Name:      testName,
						Namespace: testNs,
					},
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseDeleteInProgress,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client: tt.fields.Client,
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			}
			p.setRedisDeletionTimestampMetric(tt.args.ctx, tt.args.r)
		})
	}
}
