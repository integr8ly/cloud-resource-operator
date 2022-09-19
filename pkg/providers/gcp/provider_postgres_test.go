package gcp

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestNewGCPPostgresProvider(t *testing.T) {
	type args struct {
		client client.Client
	}
	tests := []struct {
		name string
		args args
		want *PostgresProvider
	}{
		{
			name: "placeholder test",
			args: args{},
			want: &PostgresProvider{
				Client:            nil,
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     NewDefaultConfigManager(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGCPPostgresProvider(tt.args.client); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewGCPPostgresProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_ReconcilePostgres(t *testing.T) {
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		p   *v1alpha1.Postgres
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		postgresInstance *providers.PostgresInstance
		statusMessage    types.StatusMessage
		wantErr          bool
	}{
		{
			name: "failure creating postgres instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
			},
			postgresInstance: nil,
			statusMessage:    "failed to reconcile gcp postgres provider credentials for postgres instance " + postgresProviderName,
			wantErr:          true,
		},
		{
			name: "success creating postgres instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj runtime.Object) error {
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
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
			},
			postgresInstance: nil,
			statusMessage:    "",
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := NewGCPPostgresProvider(tt.fields.Client)
			postgresInstance, statusMessage, err := pp.ReconcilePostgres(tt.args.ctx, tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcilePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(postgresInstance, tt.postgresInstance) {
				t.Errorf("ReconcilePostgres() postgresInstance = %v, want %v", postgresInstance, tt.postgresInstance)
			}
			if statusMessage != tt.statusMessage {
				t.Errorf("ReconcilePostgres() statusMessage = %v, want %v", statusMessage, tt.statusMessage)
			}
		})
	}
}

func TestPostgresProvider_DeletePostgres(t *testing.T) {
	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
	}
	type args struct {
		ctx context.Context
		p   *v1alpha1.Postgres
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
			name: "failure deleting postgres instance",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
			},
			want:    "failed to reconcile gcp postgres provider credentials for postgres instance " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "success deleting postgres instance",
			fields: fields{
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj runtime.Object) error {
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
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
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
			pp := NewGCPPostgresProvider(tt.fields.Client)
			statusMessage, err := pp.DeletePostgres(tt.args.ctx, tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeletePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if statusMessage != tt.want {
				t.Errorf("DeletePostgres() statusMessage = %v, want %v", statusMessage, tt.want)
			}
		})
	}
}
