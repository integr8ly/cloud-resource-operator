package gcp

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNs   = "testNs"
	testName = "testName"
)

func TestNewCredentialMinterCredentialManager(t *testing.T) {
	type args struct {
		client client.Client
	}
	tests := []struct {
		name string
		args args
		want *CredentialMinterCredentialManager
	}{
		{
			name: "placeholder test",
			args: args{},
			want: &CredentialMinterCredentialManager{
				ProviderCredentialName: defaultProviderCredentialName,
				Client:                 nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCredentialMinterCredentialManager(tt.args.client); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewCredentialMinterCredentialManager() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialMinterCredentialManager_ReconcileProviderCredentials(t *testing.T) {
	type fields struct {
		ProviderCredentialName string
		Client                 client.Client
	}
	type args struct {
		ctx context.Context
		ns  string
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
		want    *Credentials
		wantErr bool
	}{
		{
			name: "success reconciling provider credentials",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
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
				ns:  testNs,
			},
			want: &Credentials{
				ServiceAccountID:   "serviceAccountID",
				ServiceAccountJson: []byte("{}"),
			},
			wantErr: false,
		},
		{
			name: "failure reconciling provider credentials",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client:                 moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx: context.TODO(),
				ns:  testNs,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &CredentialMinterCredentialManager{
				ProviderCredentialName: tt.fields.ProviderCredentialName,
				Client:                 tt.fields.Client,
			}
			got, err := m.ReconcileProviderCredentials(tt.args.ctx, tt.args.ns)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileProviderCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileProviderCredentials() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialMinterCredentialManager_ReconcileCredentials(t *testing.T) {
	type fields struct {
		ProviderCredentialName string
		Client                 client.Client
	}
	type args struct {
		ctx   context.Context
		name  string
		ns    string
		roles []string
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	tests := []struct {
		name               string
		fields             fields
		args               args
		credentialsRequest *cloudcredentialv1.CredentialsRequest
		credentials        *Credentials
		mockFn             func()
		wantErr            bool
	}{
		{
			name: "success reconciling credential request",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
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
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: operatorRoles,
			},
			credentialsRequest: &cloudcredentialv1.CredentialsRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultProviderCredentialName,
					Namespace: testNs,
				},
				Status: cloudcredentialv1.CredentialsRequestStatus{
					Provisioned: true,
				},
			},
			credentials: &Credentials{
				ServiceAccountID:   "serviceAccountID",
				ServiceAccountJson: []byte("{}"),
			},
			wantErr: false,
		},
		{
			name: "failure reconciling credential request",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client:                 moqClient.NewSigsClientMoqWithScheme(runtime.NewScheme()),
			},
			args: args{
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: nil,
			},
			credentialsRequest: nil,
			credentials:        nil,
			wantErr:            true,
		},
		{
			name: "failure reconciling credentials request (generic error)",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							if cr.Status.Provisioned {
								return errors.New("generic error")
							}
							cr.Status.Provisioned = true
						}
						return nil
					}
					return mc
				}(),
			},
			args: args{
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: nil,
			},
			credentialsRequest: nil,
			credentials:        nil,
			wantErr:            true,
		},
		{
			name: "failure reconciling credentials request (not found)",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							if cr.Status.Provisioned {
								return k8serr.NewNotFound(schema.GroupResource{}, "CredentialsRequest")
							}
							cr.Status.Provisioned = true
						}
						return nil
					}
					return mc
				}(),
			},
			args: args{
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: nil,
			},
			credentialsRequest: nil,
			credentials:        nil,
			mockFn: func() {
				timeOut = time.Millisecond * 10
			},
			wantErr: true,
		},
		{
			name: "failure reconciling gcp credentials (secret error)",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							cr.Status.Provisioned = true
							cr.Status.ProviderStatus = &runtime.RawExtension{Raw: []byte("{ \"serviceAccountID\":\"serviceAccountID\"}")}
						case *corev1.Secret:
							return errors.New("generic error")
						}
						return nil
					}
					return mc
				}(),
			},
			args: args{
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: nil,
			},
			credentialsRequest: nil,
			credentials:        nil,
			wantErr:            true,
		},
		{
			name: "failure reconciling gcp credentials (secret incomplete)",
			fields: fields{
				ProviderCredentialName: defaultProviderCredentialName,
				Client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.CreateFunc = func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
						return nil
					}
					mc.UpdateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
						return nil
					}
					mc.GetFunc = func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						switch cr := obj.(type) {
						case *cloudcredentialv1.CredentialsRequest:
							cr.Status.Provisioned = true
							cr.Status.ProviderStatus = &runtime.RawExtension{Raw: []byte("{ \"serviceAccountID\":\"serviceAccountID\"}")}
						case *corev1.Secret:
							cr.Data = map[string][]byte{defaultCredentialsServiceAccount: []byte("")}
						}
						return nil
					}
					return mc
				}(),
			},
			args: args{
				ctx:   context.TODO(),
				name:  defaultProviderCredentialName,
				ns:    testNs,
				roles: nil,
			},
			credentialsRequest: nil,
			credentials:        nil,
			wantErr:            true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockFn != nil {
				tt.mockFn()
				// Reset
				defer func() {
					timeOut = time.Minute * 5
				}()
			}
			m := &CredentialMinterCredentialManager{
				ProviderCredentialName: tt.fields.ProviderCredentialName,
				Client:                 tt.fields.Client,
			}
			credentialsRequest, credentials, err := m.ReconcileCredentials(tt.args.ctx, tt.args.name, tt.args.ns, tt.args.roles)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if credentialsRequest != nil &&
				(credentialsRequest.Name != tt.credentialsRequest.Name ||
					credentialsRequest.Namespace != tt.credentialsRequest.Namespace ||
					credentialsRequest.Status.Provisioned != tt.credentialsRequest.Status.Provisioned) {
				t.Errorf("ReconcileCredentials() credentialsRequest = %v, want %v", credentialsRequest, tt.credentialsRequest)
			}
			if !reflect.DeepEqual(credentials, tt.credentials) {
				t.Errorf("ReconcileCredentials() credentials = %v, want %v", credentials, tt.credentials)
			}
		})
	}
}
