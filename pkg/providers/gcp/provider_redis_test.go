package gcp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewGCPRedisProvider(t *testing.T) {
	type args struct {
		client client.Client
	}
	tests := []struct {
		name string
		args args
		want *RedisProvider
	}{
		{
			name: "placeholder test",
			args: args{},
			want: &RedisProvider{
				Client:            nil,
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     NewDefaultConfigManager(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGCPRedisProvider(tt.args.client); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewGCPRedisProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisProvider_ReconcileRedis(t *testing.T) {
	type fields struct {
		Client            client.Client
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
			name: "failure creating redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
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
			statusMessage: "failed to reconcile gcp redis provider credentials for redis instance " + redisProviderName,
			wantErr:       true,
		},
		{
			name: "success creating redis instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							cr.Status.Provisioned = true
							cr.Status.ProviderStatus = &runtime.RawExtension{Raw: []byte("{ \"serviceAccountID\":\"serviceAccountID\"}")}
						case *corev1.Secret:
							cr.Data = map[string][]byte{defaultCredentialsServiceAccount: []byte("{}")}
						}
						return nil
					}
					return mc
				}(),
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
			statusMessage: "",
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := NewGCPRedisProvider(tt.fields.Client)
			redisCluster, statusMessage, err := rp.CreateRedis(tt.args.ctx, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRedis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(redisCluster, tt.redisCluster) {
				t.Errorf("CreateRedis() redisCluster = %v, want %v", redisCluster, tt.redisCluster)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("CreateRedis() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestRedisProvider_DeleteRedis(t *testing.T) {
	type fields struct {
		Client            client.Client
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
		name    string
		fields  fields
		args    args
		want    types.StatusMessage
		wantErr bool
	}{
		{
			name: "failure deleting redis instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
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
			want:    "failed to reconcile gcp redis provider credentials for redis instance " + redisProviderName,
			wantErr: true,
		},
		{
			name: "success deleting redis instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							cr.Status.Provisioned = true
							cr.Status.ProviderStatus = &runtime.RawExtension{Raw: []byte("{ \"serviceAccountID\":\"serviceAccountID\"}")}
						case *corev1.Secret:
							cr.Data = map[string][]byte{defaultCredentialsServiceAccount: []byte("{}")}
						}
						return nil
					}
					return mc
				}(),
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
			want:    "",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := NewGCPRedisProvider(tt.fields.Client)
			statusMessage, err := rp.DeleteRedis(tt.args.ctx, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteRedis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if statusMessage != tt.want {
				t.Errorf("DeleteRedis() statusMessage = %v, want %v", statusMessage, tt.want)
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
