package gcp

import (
	"context"
	"fmt"
	croApis "github.com/integr8ly/cloud-resource-operator/apis"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinery "k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewGCPPostgresProvider(t *testing.T) {
	type args struct {
		client client.Client
		logger *logrus.Entry
	}
	tests := []struct {
		name string
		args args
		want *PostgresProvider
	}{
		{
			name: "placeholder test",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			want: &PostgresProvider{
				Client:            nil,
				CredentialManager: NewCredentialMinterCredentialManager(nil),
				ConfigManager:     NewDefaultConfigManager(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGCPPostgresProvider(tt.args.client, tt.args.logger); !reflect.DeepEqual(got, tt.want) {
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
		Logger            *logrus.Entry
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
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
				Logger: logrus.NewEntry(logrus.StandardLogger()),
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
			pp := NewGCPPostgresProvider(tt.fields.Client, tt.fields.Logger)
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

func buildCloudSQLServiceMock(t *testing.T, reqs ...*Request) *sqladmin.Service {
	ctx := context.Background()
	svc, cleanup, err := NewSQLAdminService(
		ctx,
		reqs...,
	)
	if err != nil {
		t.Fatalf("%s", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	return svc
}

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := apimachinery.NewScheme()
	err := corev1.AddToScheme(scheme)
	err = croApis.AddToScheme(scheme)
	return scheme, err
}

func TestPostgresProvider_DeletePostgres(t *testing.T) {

	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client            client.Client
		CredentialManager CredentialManager
		ConfigManager     ConfigManager
		Logger            *logrus.Entry
	}
	type args struct {
		ctx             context.Context
		p               *v1alpha1.Postgres
		sqladminService *sqladmin.Service
	}
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
				Client:            moqClient.NewSigsClientMoqWithScheme(scheme),
				Logger:            logrus.NewEntry(logrus.StandardLogger()),
				CredentialManager: &CredentialManagerMock{},
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
				sqladminService: buildCloudSQLServiceMock(t, ListInstanceSuccess(nil)),
			},
			want:    "failed to reconcile gcp postgres provider credentials for postgres instance " + postgresProviderName,
			wantErr: true,
		},
		{
			name: "successful run of delete function when cloudsql object is already deleted",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&v1alpha1.Postgres{
						ObjectMeta: metav1.ObjectMeta{
							Name:      postgresProviderName,
							Namespace: testNs,
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				sqladminService: buildCloudSQLServiceMock(t, ListInstanceSuccess(nil)),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "successful run of delete function when cloudsql object is not already deleted",
			fields: fields{
				Client: moqClient.NewSigsClientMoqWithScheme(scheme, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
					Name:      postgresProviderName + defaultCredSecSuffix,
					Namespace: testNs,
				},
				},
					&v1alpha1.Postgres{
						ObjectMeta: metav1.ObjectMeta{
							Name:      postgresProviderName,
							Namespace: testNs,
							Annotations: map[string]string{
								ResourceIdentifierAnnotation: "testcloudsqldb-id",
							},
						},
					}),
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
						Annotations: map[string]string{
							ResourceIdentifierAnnotation: "testcloudsqldb-id",
						},
					},
				},
				sqladminService: buildCloudSQLServiceMock(t, ListInstanceSuccess(nil)),
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "want error when no annotation on postgres cr",
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
				Logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			args: args{
				ctx: context.TODO(),
				p: &v1alpha1.Postgres{
					ObjectMeta: metav1.ObjectMeta{
						Name:      postgresProviderName,
						Namespace: testNs,
					},
				},
				sqladminService: buildCloudSQLServiceMock(t, ListInstanceSuccess(nil)),
			},
			want:    "unable to find instance name from annotation",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{
				Client:            tt.fields.Client,
				Logger:            tt.fields.Logger,
				CredentialManager: tt.fields.CredentialManager,
				ConfigManager:     tt.fields.ConfigManager,
			}
			got, err := pp.deleteCloudSQLInstance(tt.args.ctx, tt.args.sqladminService, tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeletePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DeletePostgres() statusMessage = %v, want %v", got, tt.want)
			}
		})
	}
}
func ListInstanceSuccess(items []*sqladmin.DatabaseInstance) *Request {
	dbs := &sqladmin.InstancesListResponse{}
	if items != nil {
		dbs.Items = items
	} else {
		dbs.Items = []*sqladmin.DatabaseInstance{
			{
				Name: "testcloudsqldb-id",
			},
		}
	}

	r := &Request{
		reqMethod: http.MethodGet,
		reqPath:   fmt.Sprintf("/sql/v1beta4/projects/%s/instances?alt=json&prettyPrint=false", "rhoam-317914"),
		handle: func(resp http.ResponseWriter, req *http.Request) {
			b, err := dbs.MarshalJSON()
			if err != nil {
				http.Error(resp, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.WriteHeader(http.StatusOK)
			resp.Write(b)
		},
		reqCt: 1,
	}
	return r
}

func TestPostgresProvider_GetName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "success getting postgres provider name",
			want: postgresProviderName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{}
			if got := pp.GetName(); got != tt.want {
				t.Errorf("GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_SupportsStrategy(t *testing.T) {
	type args struct {
		deploymentStrategy string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "postgres provider supports strategy",
			args: args{
				deploymentStrategy: providers.GCPDeploymentStrategy,
			},
			want: true,
		},
		{
			name: "postgres provider does not support strategy",
			args: args{
				deploymentStrategy: providers.AWSDeploymentStrategy,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := PostgresProvider{}
			if got := pp.SupportsStrategy(tt.args.deploymentStrategy); got != tt.want {
				t.Errorf("SupportsStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPostgresProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "get postgres default reconcile time",
			args: args{
				p: &v1alpha1.Postgres{
					Status: types.ResourceTypeStatus{
						Phase: types.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
		{
			name: "get postgres non-default reconcile time",
			args: args{
				p: &v1alpha1.Postgres{
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
			pp := PostgresProvider{}
			if got := pp.GetReconcileTime(tt.args.p); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func NewSQLAdminService(ctx context.Context, reqs ...*Request) (*sqladmin.Service, func() error, error) {
	mc, url, cleanup := httpClient(reqs...)
	client, err := sqladmin.NewService(
		ctx,
		option.WithHTTPClient(mc),
		option.WithEndpoint(url),
	)
	return client, cleanup, err
}

func httpClient(requests ...*Request) (*http.Client, string, func() error) {
	// Create a TLS Server that responses to the requests defined
	s := httptest.NewServer(http.HandlerFunc(
		func(resp http.ResponseWriter, req *http.Request) {
			for _, r := range requests {
				if r.matches(req) {
					r.handle(resp, req)
					return
				}
			}
			// Unexpected requests should throw an error
			resp.WriteHeader(http.StatusNotImplemented)
			// TODO: follow error format better?
			resp.Write([]byte(fmt.Sprintf("unexpected request sent to mock client: %v", req)))
		},
	))
	// cleanup stops the test server and checks for uncalled requests
	cleanup := func() error {
		s.Close()
		for i, e := range requests {
			if e.reqCt > 0 {
				return fmt.Errorf("%d calls left for specified call in pos %d: %v", e.reqCt, i, e)
			}
		}
		return nil
	}

	return s.Client(), s.URL, cleanup

}

type Request struct {
	sync.Mutex

	reqMethod string
	reqPath   string
	reqCt     int

	handle func(resp http.ResponseWriter, req *http.Request)
}

// matches returns true if a given http.Request should be handled by this Request.
func (r *Request) matches(hR *http.Request) bool {
	r.Lock()
	defer r.Unlock()
	if r.reqMethod != "" && strings.ToLower(r.reqMethod) != strings.ToLower(hR.Method) {
		return false
	}
	if r.reqPath != "" && r.reqPath != hR.URL.Path {
		return false
	}
	if r.reqCt <= 0 {
		return false
	}
	r.reqCt--
	return true
}
