package openshift

import (
	"context"
	"fmt"
	"reflect"
	"testing"

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
	testPostgresName      = "test-redis"
	testPostgresNamespace = "test-redis"
)

func buildTestPostgresCR() *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      testPostgresNamespace,
			Namespace: testRedisNamespace,
		},
		Spec:   v1alpha1.PostgresSpec{},
		Status: v1alpha1.PostgresStatus{},
	}
}

func buildPostgresDeploymentReady() *appsv1.Deployment {
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

func buildTestPostgresInstance() *providers.PostgresInstance {
	return &providers.PostgresInstance{
		DeploymentDetails: &OpenShiftPostgresDeploymentDetails{
			Connection: map[string][]byte{
				"user":     []byte(defaultPostgresUser),
				"password": []byte(defaultPostgresPassword),
				"uri":      []byte(fmt.Sprintf("%s.%s.svc.cluster.local", testPostgresName, testPostgresNamespace)),
				"database": []byte(testPostgresName),
			},
		},
	}
}

func TestOpenShiftPostgresProvider_CreatePostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		logrus.Fatal(err)
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
		want    *providers.PostgresInstance
		wantErr bool
	}{
		{
			name: "test successful creation",
			fields: fields{
				Client:        fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
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
				Client:        fake.NewFakeClientWithScheme(scheme, buildPostgresDeploymentReady(), buildTestPostgresCR()),
				Logger:        testLogger,
				ConfigManager: buildDefaultConfigManager(),
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
			}
			got, err := p.CreatePostgres(tt.args.ctx, tt.args.postgres)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreatePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreatePostgres() got = %v, want %v", got, tt.want)
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

	type fields struct {
		Client        client.Client
		Logger        *logrus.Entry
		ConfigManager ConfigManager
	}
	type args struct {
		ctx      context.Context
		postgres *v1alpha1.Postgres
		object   interface{}
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			name: "test override pvc defaults",
			fields: fields{
				Client: fake.NewFakeClientWithScheme(scheme, buildTestPostgresCR(), &v1.PersistentVolumeClaim{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PersistentVolumeClaim",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPostgresName,
						Namespace: testPostgresNamespace,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								"storage": resource.MustParse("1Gi"),
							},
						},
					},
				}),
				Logger: testLogger,
				ConfigManager: &ConfigManagerMock{
					ReadStorageStrategyFunc: func(ctx context.Context, rt providers.ResourceType, tier string) (config *StrategyConfig, e error) {
						return &StrategyConfig{
							RawStrategy: []byte("{\"pvcSpec\":{\"accessModes\":[\"ReadWriteOnce\"],\"resources\":{\"requests\":{\"storage\":\"5Gi\"}},\"dataSource\":null}}"),
						}, nil
						//return &StrategyConfig{RawStrategy: json.RawMessage(`{ "pvcSpec": {"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"5Gi"}},"dataSource":null}, "secretData": {"password":"c2VjcmV0","user":"ZGltaXRyYQ=="} } `)}, nil
					},
				},
			},
			args: args{
				ctx:      context.TODO(),
				postgres: buildTestPostgresCR(),
				object:   v1.PersistentVolume{},
			},
			want: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						"storage": resource.MustParse("5Gi"),
					},
				},
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
			_, err := p.CreatePostgres(tt.args.ctx, tt.args.postgres)
			if (err != nil) != tt.wantErr {
				t.Errorf("overrideDefaults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			switch tt.want.(type) {
			case *v1.PersistentVolumeClaimSpec:
				got := &v1.PersistentVolumeClaim{}
				err = tt.fields.Client.Get(tt.args.ctx, types.NamespacedName{Name: testPostgresName, Namespace: testPostgresNamespace}, got)
				if (err != nil) != tt.wantErr {
					t.Errorf("overrideDefaults() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got.Spec, tt.want) {
					t.Errorf("overrideDefaults() \ngot = %+v, \nwant %+v", got.Spec, tt.want)
				}
			}

		})
	}
}
