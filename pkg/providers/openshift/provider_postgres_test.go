package openshift

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	types2 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	testPostgresName      = "test-postgres"
	testPostgresNamespace = "test-postgres"
	testPostgresDatabase  = "test-postgres"
	testPostgresUser      = "test-user"
	testPostgresPassword  = "test-password"
)

func buildTestPostgresCR() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      testPostgresName,
			Namespace: testPostgresNamespace,
		},
		Spec:   v1alpha1.PostgresSpec{},
		Status: v1alpha1.PostgresStatus{},
	}
}

func buildTestPostgresPVC() *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPostgresName,
			Namespace: testPostgresNamespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					"storage": resource.MustParse("5Gi"),
				},
			},
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: "bound",
		},
	}

}

func buildTestPostgresDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPostgresName,
			Namespace: testPostgresNamespace,
		},
	}
}

func buildTestPostgresDeploymentReady() *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPostgresName,
			Namespace: testPostgresNamespace,
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: "True",
				},
			},
		},
	}
}

func buildTestCredsSecret() *v1.Secret {
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", testPostgresName, defaultCredentialsSec),
			Namespace: testPostgresNamespace,
		},
		Data: map[string][]byte{
			"user":     []byte(testPostgresUser),
			"password": []byte(testPostgresPassword),
			"database": []byte(testPostgresDatabase),
		},
	}
}

func buildTestPostgresInstance() *providers.PostgresInstance {
	return &providers.PostgresInstance{
		DeploymentDetails: &providers.PostgresDeploymentDetails{
			Username: testPostgresUser,
			Password: testPostgresPassword,
			Database: testPostgresDatabase,
			Port:     defaultPostgresPort,
			Host:     fmt.Sprintf("%s.%s.svc.cluster.local", testPostgresName, testPostgresNamespace),
		},
	}
}

func buildTestConfigManager(strategy string) *ConfigManagerMock {
	return &ConfigManagerMock{
		ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (config *StrategyConfig, e error) {
			return &StrategyConfig{
				RawStrategy: []byte(strategy),
			}, nil
		},
	}
}

func buildTestPodCommander() resources.PodCommander {
	return &resources.PodCommanderMock{
		ExecIntoPodFunc: func(dpl *appsv1.Deployment, cmd string) error {
			return nil
		},
	}
}

func TestOpenShiftPostgresProvider_CreatePostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
		PodCommander  resources.PodCommander
	}
	type args struct {
		ctx      context.Context
		postgres *v1alpha1.Postgres
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *providers.PostgresInstance
		wantErr bool
	}{
		{
			name: "test successful creation",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
				PodCommander:  buildTestPodCommander(),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "test successful creation with deployment ready",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresDeploymentReady(), buildTestPostgresCR(), buildTestCredsSecret()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
				PodCommander:  buildTestPodCommander(),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want:    buildTestPostgresInstance(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftPostgresProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
				PodCommander:  tt.fields.PodCommander,
			}
			got, _, err := p.CreatePostgres(tt.args.ctx, tt.args.postgres)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreatePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreatePostgres() got = %+v, want %+v", got.DeploymentDetails, tt.want.DeploymentDetails)
			}
		})
	}
}

func TestOpenShiftPostgresProvider_DeletePostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		ctx      context.Context
		postgres *v1alpha1.Postgres
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test successful delete",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresDeploymentReady(), buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: nil,
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
		},
		{
			name: "test delete when deployment not ready",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresDeployment(), buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: nil,
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftPostgresProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
			}
			if _, err := p.DeletePostgres(tt.args.ctx, tt.args.postgres); (err != nil) != tt.wantErr {
				t.Errorf("DeletePostgres() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenShiftPostgresProvider_overrideDefaults(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	// override default config for these objects
	pvcSpec := `{"pvcSpec":{"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"5Gi"}},"dataSource":null}}`
	serviceSpec := `{"serviceSpec":{"selector":{"deployment":"updated-deployment"}}}`
	secretSpec := `{"secretData":{"user":"test-user","password":"test-password"}}`
	depSpec := `{"deploymentSpec":{"selector":{"matchLabels":{"deployment":"updated-deployment"}},"template":{"metadata":{"creationTimestamp":null},"spec":{"containers":null}},"strategy":{}}}`

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		ctx      context.Context
		postgres *v1alpha1.Postgres
	}

	tests := []struct {
		name            string
		fields          fields
		args            args
		getTestableSpec func(ctx context.Context, c client.Client) (interface{}, error)
		want            interface{}
		wantErr         bool
	}{
		{
			name: "test override pvc defaults",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), buildTestPostgresPVC()),
				Logger:        testLogger,
				ConfigManager: buildTestConfigManager(pvcSpec),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						"storage": resource.MustParse("5Gi"),
					},
				},
			},
			getTestableSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				pvc := &v1.PersistentVolumeClaim{}
				err := c.Get(ctx, types.NamespacedName{Name: testPostgresName, Namespace: testPostgresNamespace}, pvc)
				return pvc.Spec, err
			},
			wantErr: false,
		},
		{
			name: "test override secret defaults",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildTestConfigManager(secretSpec),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want: map[string]string{
				"user":     "test-user",
				"password": "test-password",
			},
			getTestableSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				sec := &v1.Secret{}
				err := c.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", testPostgresName, defaultCredentialsSec), Namespace: testPostgresNamespace}, sec)
				return sec.StringData, err
			},
			wantErr: false,
		},
		{
			name: "test override deployment defaults",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildTestConfigManager(depSpec),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"deployment": "updated-deployment",
					},
				},
			},
			getTestableSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				depl := &appsv1.Deployment{}
				err := c.Get(ctx, types.NamespacedName{Name: testPostgresName, Namespace: testPostgresNamespace}, depl)
				return depl.Spec, err
			},
			wantErr: false,
		},
		{
			name: "test override service defaults",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildTestConfigManager(serviceSpec),
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
			},
			want: v1.ServiceSpec{
				Selector: map[string]string{"deployment": "updated-deployment"},
			},
			getTestableSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				svc := &v1.Service{}
				err := c.Get(ctx, types.NamespacedName{Name: testPostgresName, Namespace: testPostgresNamespace}, svc)
				return svc.Spec, err
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftPostgresProvider{
				Client:        tt.fields.Client,
				Logger:        tt.fields.Logger,
				ConfigManager: tt.fields.ConfigManager,
			}
			_, _, err := p.CreatePostgres(tt.args.ctx, tt.args.postgres)
			if (err != nil) != tt.wantErr {
				t.Errorf("overrideDefaults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got, err := tt.getTestableSpec(tt.args.ctx, tt.fields.Client)
			if err != nil {
				t.Error("overrideDefaults() unexpected error while getting testable spec", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("overrideDefaults() \n got = %+v, \n want = %+v", got, tt.want)
			}
		})
	}
}

func TestOpenShiftPostgresProvider_GetReconcileTime(t *testing.T) {
	type args struct {
		p *v1alpha1.Postgres
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		{
			name: "test short reconcile when the cr is not complete",
			args: args{
				p: &v1alpha1.Postgres{
					Status: v1alpha1.PostgresStatus{
						Phase: types2.PhaseInProgress,
					},
				},
			},
			want: time.Second * 10,
		},
		{
			name: "test default reconcile time when the cr is complete",
			args: args{
				p: &v1alpha1.Postgres{
					Status: v1alpha1.PostgresStatus{
						Phase: types2.PhaseComplete,
					},
				},
			},
			want: defaultReconcileTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &OpenShiftPostgresProvider{}
			if got := p.GetReconcileTime(tt.args.p); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
