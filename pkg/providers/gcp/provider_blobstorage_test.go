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

func TestNewGCPBlobStorageProvider(t *testing.T) {
	type args struct {
		client client.Client
	}
	tests := []struct {
		name string
		args args
		want *BlobStorageProvider
	}{
		{
			name: "placeholder test",
			args: args{},
			want: &BlobStorageProvider{
				Client:            nil,
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     NewDefaultConfigManager(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGCPBlobStorageProvider(tt.args.client); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewGCPBlobStorageProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_CreateStorage(t *testing.T) {
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		bs  *v1alpha1.BlobStorage
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	tests := []struct {
		name                string
		fields              fields
		args                args
		blobStorageInstance *providers.BlobStorageInstance
		statusMessage       types.StatusMessage
		wantErr             bool
	}{
		{
			name: "failure creating blob storage",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx: context.TODO(),
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      blobstorageProviderName,
						Namespace: testNs,
					},
				},
			},
			blobStorageInstance: nil,
			statusMessage:       "failed to reconcile gcp blob storage provider credentials for blob storage instance " + blobstorageProviderName,
			wantErr:             true,
		},
		{
			name: "success creating blob storage",
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
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      blobstorageProviderName,
						Namespace: testNs,
					},
				},
			},
			blobStorageInstance: nil,
			statusMessage:       "",
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bsp := NewGCPBlobStorageProvider(tt.fields.Client)
			blobStorageInstance, statusMessage, err := bsp.CreateStorage(tt.args.ctx, tt.args.bs)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(blobStorageInstance, tt.blobStorageInstance) {
				t.Errorf("CreateStorage() blobStorageInstance = %v, want %v", blobStorageInstance, tt.blobStorageInstance)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("CreateStorage() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestBlobStorageProvider_DeleteStorage(t *testing.T) {
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		bs  *v1alpha1.BlobStorage
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
			name: "failure deleting blob storage",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx: context.TODO(),
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      blobstorageProviderName,
						Namespace: testNs,
					},
				},
			},
			want:    "failed to reconcile gcp blob storage provider credentials for blob storage instance " + blobstorageProviderName,
			wantErr: true,
		},
		{
			name: "success deleting blob storage",
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
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      blobstorageProviderName,
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
			bsp := NewGCPBlobStorageProvider(tt.fields.Client)
			statusMessage, err := bsp.DeleteStorage(tt.args.ctx, tt.args.bs)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if statusMessage != tt.want {
				t.Errorf("DeleteStorage() statusMessage = %v, want %v", statusMessage, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_GetName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "success getting blob storage provider name",
			want: blobstorageProviderName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bsp := BlobStorageProvider{}
			if got := bsp.GetName(); got != tt.want {
				t.Errorf("GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_SupportsStrategy(t *testing.T) {
	type args struct {
		deploymentStrategy string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "blob storage provider supports strategy",
			args: args{
				deploymentStrategy: providers.GCPDeploymentStrategy,
			},
			want: true,
		},
		{
			name: "blob storage provider does not support strategy",
			args: args{
				deploymentStrategy: providers.AWSDeploymentStrategy,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bsp := BlobStorageProvider{}
			if got := bsp.SupportsStrategy(tt.args.deploymentStrategy); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		bs *v1alpha1.BlobStorage
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "get blob storage default reconcile time",
			args: args{
				bs: &v1alpha1.BlobStorage{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
		{
			name: "get blob storage non-default reconcile time",
			args: args{
				bs: &v1alpha1.BlobStorage{
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
			bsp := BlobStorageProvider{}
			if got := bsp.GetReconcileTime(tt.args.bs); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
