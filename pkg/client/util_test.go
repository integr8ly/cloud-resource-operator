package client

import (
	"context"
	"errors"
	"reflect"
	"testing"

	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/integr8ly/cloud-resource-operator/apis"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := apis.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func TestReconcileBlobStorage(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type args struct {
		ctx            context.Context
		client         client.Client
		productName    string
		deploymentType string
		tier           string
		name           string
		ns             string
		secretName     string
		secretNs       string
		modifyFunc     modifyResourceFunc
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.BlobStorage
		wantErr bool
	}{
		{
			name: "test successful creation",
			args: args{
				ctx:            context.TODO(),
				client:         fake.NewFakeClientWithScheme(scheme),
				deploymentType: "managed",
				tier:           "production",
				productName:    "test",
				name:           "test",
				ns:             "test",
				secretName:     "test",
				secretNs:       "test",
				modifyFunc:     nil,
			},
			want: &v1alpha1.BlobStorage{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function",
			args: args{
				ctx:            context.TODO(),
				client:         fake.NewFakeClientWithScheme(scheme),
				deploymentType: "managed",
				tier:           "production",
				name:           "test",
				productName:    "test",
				ns:             "test",
				secretName:     "test",
				secretNs:       "test",
				modifyFunc: func(cr v1.Object) error {
					cr.SetLabels(map[string]string{
						"cro": "test",
					})
					return nil
				},
			},
			want: &v1alpha1.BlobStorage{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"cro": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function error",
			args: args{
				ctx:            context.TODO(),
				client:         fake.NewFakeClientWithScheme(scheme),
				deploymentType: "workshop",
				tier:           "development",
				name:           "test",
				productName:    "test",
				ns:             "test",
				secretName:     "test",
				secretNs:       "test",
				modifyFunc: func(cr v1.Object) error {
					return errors.New("error executing function")
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileBlobStorage(tt.args.ctx, tt.args.client, tt.args.productName, tt.args.deploymentType, tt.args.tier, tt.args.name, tt.args.ns, tt.args.secretName, tt.args.secretNs, tt.args.modifyFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBlobStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileBlobStorage() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcilePostgres(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	upgradePostgres := &v1alpha1.Postgres{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "test",
			ResourceVersion: "0",
		},
	}

	type args struct {
		ctx              context.Context
		client           client.Client
		deploymentType   string
		productName      string
		tier             string
		name             string
		ns               string
		secretName       string
		secretNs         string
		applyImmediately bool
		modifyFunc       modifyResourceFunc
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.Postgres
		wantErr bool
	}{
		{
			name: "test successful creation on create",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme),
				deploymentType:   "managed",
				tier:             "production",
				productName:      "test",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: false,
				modifyFunc:       nil,
			},
			want: &v1alpha1.Postgres{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function on create",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme),
				deploymentType:   "managed",
				productName:      "test",
				tier:             "production",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: true,
				modifyFunc: func(cr v1.Object) error {
					cr.SetLabels(map[string]string{
						"productName": "test",
					})
					return nil
				},
			},
			want: &v1alpha1.Postgres{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
					ApplyImmediately: true,
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function error on create",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme),
				deploymentType:   "workshop",
				tier:             "development",
				productName:      "test",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: false,
				modifyFunc: func(cr v1.Object) error {
					return errors.New("error executing function")
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test successful creation on upgrade",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme, upgradePostgres),
				deploymentType:   "managed",
				tier:             "production",
				productName:      "test",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: false,
				modifyFunc:       nil,
			},
			want: &v1alpha1.Postgres{
				TypeMeta: v1.TypeMeta{
					Kind:       "Postgres",
					APIVersion: "integreatly.org/v1alpha1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function on upgrade",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme, upgradePostgres),
				deploymentType:   "managed",
				productName:      "test",
				tier:             "production",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: true,
				modifyFunc: func(cr v1.Object) error {
					cr.SetLabels(map[string]string{
						"productName": "test",
					})
					return nil
				},
			},
			want: &v1alpha1.Postgres{
				TypeMeta: v1.TypeMeta{
					Kind:       "Postgres",
					APIVersion: "integreatly.org/v1alpha1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
					ApplyImmediately: true,
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function error on upgrade",
			args: args{
				ctx:              context.TODO(),
				client:           fake.NewFakeClientWithScheme(scheme, upgradePostgres),
				deploymentType:   "workshop",
				tier:             "development",
				productName:      "test",
				name:             "test",
				ns:               "test",
				secretName:       "test",
				secretNs:         "test",
				applyImmediately: false,
				modifyFunc: func(cr v1.Object) error {
					return errors.New("error executing function")
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcilePostgres(tt.args.ctx, tt.args.client, tt.args.productName, tt.args.deploymentType, tt.args.tier, tt.args.name, tt.args.ns, tt.args.secretName, tt.args.secretNs, tt.args.applyImmediately, tt.args.modifyFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcilePostgres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcilePostgres() \n got = %v, \n want= %v", got, tt.want)
			}
		})
	}
}

func TestReconcileRedis(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type args struct {
		ctx               context.Context
		client            client.Client
		deploymentType    string
		tier              string
		productName       string
		name              string
		ns                string
		secretName        string
		secretNs          string
		size              string
		applyImmediately  bool
		maintenanceWindow bool
		modifyFunc        modifyResourceFunc
	}
	tests := []struct {
		name    string
		args    args
		want    *v1alpha1.Redis
		wantErr bool
	}{
		{
			name: "test successful creation",
			args: args{
				ctx:            context.TODO(),
				client:         fake.NewFakeClientWithScheme(scheme),
				deploymentType: "managed",
				tier:           "production",
				productName:    "test",
				name:           "test",
				ns:             "test",
				secretName:     "test",
				secretNs:       "test",
				modifyFunc:     nil,
			},
			want: &v1alpha1.Redis{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"productName": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function",
			args: args{
				ctx:               context.TODO(),
				client:            fake.NewFakeClientWithScheme(scheme),
				deploymentType:    "managed",
				tier:              "production",
				productName:       "test",
				name:              "test",
				ns:                "test",
				secretName:        "test",
				secretNs:          "test",
				size:              "test",
				applyImmediately:  true,
				maintenanceWindow: true,
				modifyFunc: func(cr v1.Object) error {
					cr.SetLabels(map[string]string{
						"cro": "test",
					})
					return nil
				},
			},
			want: &v1alpha1.Redis{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "test",
					ResourceVersion: "1",
					Labels: map[string]string{
						"cro": "test",
					},
				},
				Spec: croType.ResourceTypeSpec{
					Type: "managed",
					Tier: "production",
					SecretRef: &croType.SecretRef{
						Name:      "test",
						Namespace: "test",
					},
					Size:              "test",
					ApplyImmediately:  true,
					MaintenanceWindow: true,
				},
			},
			wantErr: false,
		},
		{
			name: "test modification function error",
			args: args{
				ctx:            context.TODO(),
				client:         fake.NewFakeClientWithScheme(scheme),
				deploymentType: "workshop",
				tier:           "development",
				productName:    "test",
				name:           "test",
				ns:             "test",
				secretName:     "test",
				secretNs:       "test",
				modifyFunc: func(cr v1.Object) error {
					return errors.New("error executing function")
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileRedis(tt.args.ctx, tt.args.client, tt.args.productName, tt.args.deploymentType, tt.args.tier, tt.args.name, tt.args.ns, tt.args.secretName, tt.args.secretNs, tt.args.size, tt.args.applyImmediately, tt.args.maintenanceWindow, tt.args.modifyFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileRedis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileRedis() got = %v, want %v", got, tt.want)
			}
		})
	}
}
