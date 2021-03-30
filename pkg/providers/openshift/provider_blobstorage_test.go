package openshift

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBlobStorageProvider_CreateStorage(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		ctx context.Context
		bs  *v1alpha1.BlobStorage
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.BlobStorageInstance
		wantErr bool
	}{
		{
			name: "test secret is created",
			fields: fields{
				Client: fake.NewFakeClient(),
				Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: croType.ResourceTypeSpec{
						SecretRef: &types.SecretRef{
							Name:      "test-sec",
							Namespace: "",
						},
					},
					Status: croType.ResourceTypeStatus{},
				},
			},
			want: &providers.BlobStorageInstance{
				DeploymentDetails: &BlobStorageDeploymentDetails{
					data: map[string]string{
						aws.DetailsBlobStorageCredentialSecretKey: varPlaceholder,
						aws.DetailsBlobStorageCredentialKeyID:     varPlaceholder,
						aws.DetailsBlobStorageBucketRegion:        varPlaceholder,
						aws.DetailsBlobStorageBucketName:          varPlaceholder,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test existing secret is not overridden",
			fields: fields{
				Client: fake.NewFakeClient(&v12.Secret{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "test",
						Name:      "test",
					},
					Data: map[string][]byte{
						aws.DetailsBlobStorageBucketName:          []byte("test"),
						aws.DetailsBlobStorageBucketRegion:        []byte("test"),
						aws.DetailsBlobStorageCredentialKeyID:     []byte("test"),
						aws.DetailsBlobStorageCredentialSecretKey: []byte("test"),
						"test": []byte("test"),
					},
				}),
				Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: croType.ResourceTypeSpec{
						SecretRef: &types.SecretRef{
							Name:      "test-sec",
							Namespace: "",
						},
					},
					Status: croType.ResourceTypeStatus{
						Phase: types.PhaseComplete,
						SecretRef: &types.SecretRef{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			want: &providers.BlobStorageInstance{
				DeploymentDetails: &BlobStorageDeploymentDetails{
					data: map[string]string{
						aws.DetailsBlobStorageBucketName:          "test",
						aws.DetailsBlobStorageBucketRegion:        "test",
						aws.DetailsBlobStorageCredentialKeyID:     "test",
						aws.DetailsBlobStorageCredentialSecretKey: "test",
						"test": "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test missing secret values are reset",
			fields: fields{
				Client: fake.NewFakeClient(&v12.Secret{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "test",
						Name:      "test",
					},
					Data: map[string][]byte{
						aws.DetailsBlobStorageCredentialKeyID: []byte("test"),
						"test":                                []byte("test"),
					},
				}),
				Logger: &logrus.Entry{},
			},
			args: args{
				ctx: context.TODO(),
				bs: &v1alpha1.BlobStorage{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Spec: croType.ResourceTypeSpec{
						SecretRef: &types.SecretRef{
							Name:      "test-sec",
							Namespace: "",
						},
					},
					Status: croType.ResourceTypeStatus{
						Phase: types.PhaseComplete,
						SecretRef: &types.SecretRef{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			want: &providers.BlobStorageInstance{
				DeploymentDetails: &BlobStorageDeploymentDetails{
					data: map[string]string{
						aws.DetailsBlobStorageBucketName:          varPlaceholder,
						aws.DetailsBlobStorageBucketRegion:        varPlaceholder,
						aws.DetailsBlobStorageCredentialKeyID:     "test",
						aws.DetailsBlobStorageCredentialSecretKey: varPlaceholder,
						"test": "test",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BlobStorageProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			got, _, err := b.CreateStorage(tt.args.ctx, tt.args.bs)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateStorage() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_GetReconcileTime(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		bs *v1alpha1.BlobStorage
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   time.Duration
	}{
		{
			name: "test expected value for regression",
			want: time.Second * 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BlobStorageProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			if got := b.GetReconcileTime(tt.args.bs); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlobStorageProvider_SupportsStrategy(t *testing.T) {
	type fields struct {
		Client client.Client
		Logger *logrus.Entry
	}
	type args struct {
		s string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "test success",
			args: args{s: "openshift"},
			want: true,
		},
		{
			name: "test failure",
			args: args{s: "test"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BlobStorageProvider{
				Client: tt.fields.Client,
				Logger: tt.fields.Logger,
			}
			if got := b.SupportsStrategy(tt.args.s); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}
