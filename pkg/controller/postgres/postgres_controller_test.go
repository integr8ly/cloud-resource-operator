package postgres

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
)

var (
	testPostgresName      = "test-postgres"
	testPostgresNamespace = "test-postgres"
	testLogger            = logrus.WithFields(logrus.Fields{"testing": "true"})
)

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := apis.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestPostgresCR() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      testPostgresName,
			Namespace: testPostgresNamespace,
		},
		Spec: v1alpha1.PostgresSpec{
			SecretRef: &v1alpha1.SecretRef{
				Name:      testPostgresName,
				Namespace: testPostgresName,
			},
		},
		Status: v1alpha1.PostgresStatus{},
	}
}

func TestReconcilePostgres_Reconcile(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR())

	type fields struct {
		client       client.Client
		scheme       *runtime.Scheme
		logger       *logrus.Entry
		ctx          context.Context
		providerList []providers.PostgresProvider
		cfgMgr       providers.ConfigManager
	}
	type args struct {
		request reconcile.Request
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				client: fakeClient,
				scheme: scheme,
				logger: testLogger,
				ctx:    context.TODO(),
				providerList: []providers.PostgresProvider{
					&providers.PostgresProviderMock{
						CreatePostgresFunc: func(ctx context.Context, ps *v1alpha1.Postgres) (instance *providers.PostgresInstance, msg v1alpha1.StatusMessage, e error) {
							return &providers.PostgresInstance{
								DeploymentDetails: &providers.DeploymentDetailsMock{
									DataFunc: func() map[string][]byte {
										return map[string][]byte{
											"test": []byte("test"),
										}
									},
								},
							}, "test", nil
						},
						DeletePostgresFunc: func(ctx context.Context, ps *v1alpha1.Postgres) error {
							return nil
						},
						GetNameFunc: func() string {
							return "test"
						},
						SupportsStrategyFunc: func(s string) bool {
							return s == "test"
						},
					},
				},
				cfgMgr: &providers.ConfigManagerMock{
					GetStrategyMappingForDeploymentTypeFunc: func(ctx context.Context, t string) (*providers.DeploymentStrategyMapping, error) {
						return &providers.DeploymentStrategyMapping{
							Postgres: "test",
						}, nil
					},
				},
			},
			args: args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test-postgres",
						Name:      "test-postgres",
					},
				},
			},
			wantErr: false,
			want: struct {
				Requeue      bool
				RequeueAfter time.Duration
			}{Requeue: true, RequeueAfter: 30 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcilePostgres{
				client:       tt.fields.client,
				scheme:       tt.fields.scheme,
				logger:       tt.fields.logger,
				ctx:          tt.fields.ctx,
				providerList: tt.fields.providerList,
				cfgMgr:       tt.fields.cfgMgr,
			}
			got, err := r.Reconcile(tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconcile() got = %v, want %v", got, tt.want)
			}
		})
	}
}
