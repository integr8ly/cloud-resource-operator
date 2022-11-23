package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"google.golang.org/api/servicenetworking/v1"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	redis "cloud.google.com/go/redis/apiv1"
	"github.com/googleapis/gax-go/v2"
	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	redispb "google.golang.org/genproto/googleapis/cloud/redis/v1"
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

func TestNewGCPRedisProvider(t *testing.T) {
	type args struct {
		client client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name string
		args args
		want *RedisProvider
	}{
		{
			name: "placeholder test",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			want: &RedisProvider{
				Client:            nil,
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     NewDefaultConfigManager(nil),
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
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "success triggering redis deletion for an existing instance",
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return []*redispb.Instance{
							{
								Name:  fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName),
								State: redispb.Instance_READY,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("delete detected, redis instance %s started", testName)),
			wantErr: false,
		},
		{
			name: "success reconciling when the found instance is already in progress of deletion",
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return []*redispb.Instance{
							{
								Name:  fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName),
								State: redispb.Instance_DELETING,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("deletion in progress for redis instance %s", testName)),
			wantErr: false,
		},
		{
			name: "success reconciling when the instance deletion and cleanup have completed",
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("successfully deleted gcp redis instance %s", testName)),
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
			want:    types.StatusMessage("failed to build delete redis instance request"),
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return []*redispb.Instance{
							{
								Name:  fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName),
								State: redispb.Instance_READY,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, fmt.Errorf("generic error")
					}
				}),
				strategyConfig: buildTestStrategyConfig(),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to delete redis instance %s", testName)),
			wantErr: true,
		},
		{
			name: "fail to retrieve redis instances",
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, fmt.Errorf("generic error")
					}

				}),
				strategyConfig: buildTestStrategyConfig(),
			},
			want:    "failed to retrieve redis instances",
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, nil
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

func TestRedisProvider_getRedisStrategyConfig(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		tier string
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	tests := []struct {
		name           string
		fields         fields
		args           args
		strategyConfig *StrategyConfig
		wantErr        bool
	}{
		{
			name: "successfully retrieve gcp redis config",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
					buildTestGcpStrategyConfigMap(nil),
				),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{
							CreateStrategy: json.RawMessage(`{}`),
							DeleteStrategy: json.RawMessage(`{}`),
						}, nil
					},
				},
			},
			args: args{
				tier: "development",
			},
			strategyConfig: &StrategyConfig{
				Region:         gcpTestRegion,
				ProjectID:      gcpTestProjectId,
				CreateStrategy: json.RawMessage(`{}`),
				DeleteStrategy: json.RawMessage(`{}`),
			},
			wantErr: false,
		},
		{
			name: "fail to read gcp strategy config",
			fields: fields{
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, fmt.Errorf("generic error")
					},
				},
			},
			args: args{
				tier: "development",
			},
			strategyConfig: nil,
			wantErr:        true,
		},
		{
			name: "fail to retrieve default gcp project",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(map[string]*string{"projectID": utils.String("")}),
				),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
					},
				},
			},
			args: args{
				tier: "development",
			},
			strategyConfig: nil,
			wantErr:        true,
		},
		{
			name: "fail to retrieve default gcp region",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(map[string]*string{"region": utils.String("")}),
				),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
					},
				},
			},
			args: args{
				tier: "development",
			},
			strategyConfig: nil,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			strategyConfig, err := rp.getRedisStrategyConfig(context.TODO(), tt.args.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("getRedisStrategyConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(strategyConfig, tt.strategyConfig) {
				t.Errorf("getRedisConfig() strategyConfig = %v, strategyConfig expected %v", strategyConfig, tt.strategyConfig)
			}
		})
	}
}

func TestRedisProvider_getRedisInstances(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx         context.Context
		redisClient gcpiface.RedisAPI
		projectID   string
		region      string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []*redispb.Instance
		wantErr bool
	}{
		{
			name:   "successfully retrieve redis instances",
			fields: fields{},
			args: args{
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return []*redispb.Instance{{Name: fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName)}}, nil
					}
				}),
				projectID: gcpTestProjectId,
				region:    gcpTestRegion,
			},
			want:    []*redispb.Instance{{Name: fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName)}},
			wantErr: false,
		},
		{
			name:   "fail to retrieve redis instances",
			fields: fields{},
			args: args{
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.ListInstancesFn = func(ctx context.Context, request *redispb.ListInstancesRequest, option ...gax.CallOption) ([]*redispb.Instance, error) {
						return nil, fmt.Errorf("generic error")
					}
				}),
				projectID: gcpTestProjectId,
				region:    gcpTestRegion,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := rp.getRedisInstances(tt.args.ctx, tt.args.redisClient, tt.args.projectID, tt.args.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("getRedisInstances() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getRedisInstances() got = %v, want %v", got, tt.want)
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
				Name: fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName),
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
				Name: fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, testName),
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
	networkAddress := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", gcpTestProjectId, gcpTestNetworkName)
	parent := fmt.Sprintf(redisParentFormat, gcpTestProjectId, gcpTestRegion)
	instanceID := "gcptestclustertestNstestName"
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
		r              *v1alpha1.Redis
		strategyConfig *StrategyConfig
		networkAddress string
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
				Client: nil,
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
					CreateStrategy: json.RawMessage(fmt.Sprintf(`{"parent":"%s", "instance_id": "%s", "instance": {} }`, parent, instanceID)),
				},
				networkAddress: networkAddress,
			},
			want: &redispb.CreateInstanceRequest{
				Parent:     parent,
				InstanceId: instanceID,
				Instance:   &redispb.Instance{},
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
				networkAddress: networkAddress,
			},
			want: &redispb.CreateInstanceRequest{
				Parent:     parent,
				InstanceId: instanceID,
				Instance: &redispb.Instance{
					Name:              fmt.Sprintf(redisInstanceNameFormat, gcpTestProjectId, gcpTestRegion, instanceID),
					Tier:              redispb.Instance_STANDARD_HA,
					MemorySizeGb:      redisMemorySizeGB,
					AuthorizedNetwork: strings.Split(networkAddress, "v1/")[1],
					RedisVersion:      redisVersion,
				},
			},
			wantErr: false,
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
					mockClient.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
				networkAddress: networkAddress,
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
			got, err := p.buildCreateInstanceRequest(context.TODO(), tt.args.r, tt.args.strategyConfig, tt.args.networkAddress)
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
	_ = v1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	networkAddress := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", gcpTestProjectId, gcpTestNetworkName)
	redisCR := &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNs,
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
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return nil, nil
					},
				},
			},
			args: args{
				networkManager: &NetworkManagerMock{
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
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
					Region:         gcpTestProjectId,
					ProjectID:      gcpTestRegion,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster: &providers.RedisCluster{
				DeploymentDetails: &providers.RedisDeploymentDetails{},
			},
			statusMessage: "successfully created gcp redis instance " + testName,
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return nil, fmt.Errorf("generic error")
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to create network service",
			wantErr:       true,
		},
		{
			name:   "network ip address range creation in progress",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVING.String()),
							Network: &networkAddress,
						}, nil
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "network ip address range creation in progress",
			wantErr:       false,
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return nil, fmt.Errorf("generic error")
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to create network service",
			wantErr:       true,
		},
		{
			name:   "network service connection creation in progress",
			fields: fields{},
			args: args{
				networkManager: &NetworkManagerMock{
					ReconcileNetworkProviderConfigFunc: func(ctx context.Context, configManager ConfigManager, tier string) (*net.IPNet, error) {
						return &net.IPNet{
							Mask: net.CIDRMask(defaultIpRangeCIDRMask, defaultIpv4Length),
						}, nil
					},
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return nil, nil
					},
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "network service connection creation in progress",
			wantErr:       false,
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
					},
				},
				strategyConfig: &StrategyConfig{
					CreateStrategy: nil,
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to build create redis instance request",
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
					},
				},
				redisClient: gcpiface.GetMockRedisClient(func(redisClient *gcpiface.MockRedisClient) {
					redisClient.GetInstanceFn = func(ctx context.Context, request *redispb.GetInstanceRequest, option ...gax.CallOption) (*redispb.Instance, error) {
						return nil, fmt.Errorf("generic error")
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
			statusMessage: "failed to fetch redis instance gcptestcluster",
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
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
					Region:         gcpTestProjectId,
					ProjectID:      gcpTestRegion,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{},
			},
			redisCluster:  nil,
			statusMessage: "failed to create redis instance gcptestcluster",
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
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
				r: redisCR,
			},
			redisCluster:  nil,
			statusMessage: "started creation of gcp redis instance",
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
					CreateNetworkIpRangeFunc: func(ctx context.Context, cidrRange *net.IPNet) (*computepb.Address, error) {
						return &computepb.Address{
							Status:  utils.String(computepb.Address_RESERVED.String()),
							Network: &networkAddress,
						}, nil
					},
					CreateNetworkServiceFunc: func(ctx context.Context) (*servicenetworking.Connection, error) {
						return &servicenetworking.Connection{}, nil
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
					Region:         gcpTestProjectId,
					ProjectID:      gcpTestRegion,
					CreateStrategy: json.RawMessage(`{}`),
				},
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			redisCluster:  nil,
			statusMessage: "creation in progress for redis instance " + testName,
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RedisProvider{
				Client:            tt.fields.Client,
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
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
