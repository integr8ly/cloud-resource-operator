package gcp

import (
	redis "cloud.google.com/go/redis/apiv1"
	"context"
	"fmt"
	"github.com/googleapis/gax-go/v2"
	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	redispb "google.golang.org/genproto/googleapis/cloud/redis/v1"
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
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

func TestRedisProvider_ReconcileRedis(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		r   *v1alpha1.Redis
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
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
			name: "success creating redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      redisProviderName,
						Namespace: testNs,
					},
				},
			},
			redisCluster:  nil,
			statusMessage: "successfully created gcp redis",
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := RedisProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			redisCluster, statusMessage, err := p.createRedisCluster(tt.args.ctx, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRedisCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(redisCluster, tt.redisCluster) {
				t.Errorf("createRedisCluster() redisCluster = %v, want %v", redisCluster, tt.redisCluster)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("createRedisCluster() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestRedisProvider_deleteRedisCluster(t *testing.T) {
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
		r              *v1alpha1.Redis
		isLastResource bool
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
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
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
								Name:  fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName),
								State: redispb.Instance_READY,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("delete detected, redis instance %s started", testName)),
			wantErr: false,
		},
		{
			name: "success reconciling when the found instance is already in progress of deletion",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
								Name:  fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName),
								State: redispb.Instance_DELETING,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, nil
					}
				}),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("deletion in progress for redis instance %s", testName)),
			wantErr: false,
		},
		{
			name: "success reconciling when the instance deletion and cleanup have completed",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				}),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("successfully deleted redis instance %s", testName)),
			wantErr: false,
		},
		{
			name: "fail to delete redis instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
								Name:  fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName),
								State: redispb.Instance_READY,
							},
						}, nil
					}
					redisClient.DeleteInstanceFn = func(ctx context.Context, request *redispb.DeleteInstanceRequest, option ...gax.CallOption) (*redis.DeleteInstanceOperation, error) {
						return &redis.DeleteInstanceOperation{}, fmt.Errorf("generic error")
					}
				}),
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to delete redis instance %s", testName)),
			wantErr: true,
		},
		{
			name: "fail to retrieve gcp redis strategy config",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNs,
					},
				},
				isLastResource: false,
			},
			want:    "failed to retrieve redis strategy config",
			wantErr: true,
		},
		{
			name: "fail to retrieve redis instances",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
			},
			want:    "failed to retrieve redis instances",
			wantErr: true,
		},
		{
			name: "fail to update redis instance as part of finalizer reconcile",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: false,
			},
			want:    types.StatusMessage(fmt.Sprintf("failed to update instance %s as part of finalizer reconcile", testName)),
			wantErr: true,
		},
		{
			name: "fail to delete cluster network peering",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: true,
			},
			want:    "failed to delete cluster network peering",
			wantErr: true,
		},
		{
			name: "fail to delete network service",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: true,
			},
			want:    "failed to delete network service",
			wantErr: true,
		},
		{
			name: "fail to delete network ip range",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: true,
			},
			want:    "failed to delete network ip range",
			wantErr: true,
		},
		{
			name: "successfully reconcile when components deletion is in progress",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: true,
			},
			want:    "network component deletion in progress",
			wantErr: false,
		},
		{
			name: "fail to check if components exist",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{}}`}
						}
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
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
				isLastResource: true,
			},
			want:    "failed to check if components exist",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGCPRedisProvider(tt.fields.Client, tt.fields.Logger)
			statusMessage, err := p.deleteRedisCluster(tt.args.ctx, tt.args.networkManager, tt.args.redisClient, tt.args.r, tt.args.isLastResource)
			if (err != nil) != tt.wantErr {
				t.Errorf("deleteRedisCluster() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if statusMessage != tt.want {
				t.Errorf("deleteRedisCluster() statusMessage = %v, want %v", statusMessage, tt.want)
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

func TestRedisProvider_getRedisConfig(t *testing.T) {
	type fields struct {
		Client            client.Client
		Logger            *logrus.Entry
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		r   *v1alpha1.Redis
	}
	tests := []struct {
		name                  string
		fields                fields
		args                  args
		createInstanceRequest *redispb.CreateInstanceRequest
		deleteInstanceRequest *redispb.DeleteInstanceRequest
		strategyConfig        *StrategyConfig
		wantErr               bool
	}{
		{
			name: "successfully retrieve gcp redis config",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
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
					},
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: &redispb.DeleteInstanceRequest{
				Name: fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName),
			},
			strategyConfig: &StrategyConfig{
				Region:    "region",
				ProjectID: "projectID",
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
				r: &v1alpha1.Redis{
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			strategyConfig:        nil,
			wantErr:               true,
		},
		{
			name: "fail to retrieve default gcp project",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
					},
				},
			},
			args: args{
				r: &v1alpha1.Redis{
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			strategyConfig:        nil,
			wantErr:               true,
		},
		{
			name: "fail to retrieve default gcp region",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
					},
				},
			},
			args: args{
				r: &v1alpha1.Redis{
					Spec: types.ResourceTypeSpec{
						Tier: "development",
					},
				},
			},
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			strategyConfig:        nil,
			wantErr:               true,
		},
		{
			name: "fail to retrieve redis instance from cr annotations",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
									Region:    "region",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
						return &StrategyConfig{}, nil
					},
				},
			},
			args: args{
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
			createInstanceRequest: nil,
			deleteInstanceRequest: nil,
			strategyConfig:        nil,
			wantErr:               true,
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
			createInstanceRequest, deleteInstanceRequest, strategyConfig, err := rp.getRedisConfig(tt.args.ctx, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("getRedisConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(createInstanceRequest, tt.createInstanceRequest) {
				t.Errorf("getRedisConfig() createInstanceRequest = %v, createInstanceRequest expected %v", createInstanceRequest, tt.createInstanceRequest)
			}
			if deleteInstanceRequest != nil && (deleteInstanceRequest.Name != tt.deleteInstanceRequest.Name) {
				t.Errorf("getRedisConfig() deleteInstanceRequest = %v, deleteInstanceRequest expected %v", deleteInstanceRequest.Name, tt.deleteInstanceRequest.Name)
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
						return []*redispb.Instance{{Name: fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName)}}, nil
					}
				}),
				projectID: "projectID",
				region:    "region",
			},
			want:    []*redispb.Instance{{Name: fmt.Sprintf(redisInstanceNameFormat, "projectID", "region", testName)}},
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
				projectID: "projectID",
				region:    "region",
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
