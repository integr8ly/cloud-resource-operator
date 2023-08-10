package resources

import (
	"context"
	"reflect"
	"testing"

	croapis "github.com/integr8ly/cloud-resource-operator/apis"
	v1alpha1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-credential-operator/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildTestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	err := croapis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = configv1.Install(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}

func buildTestPostgresCR(maintenanceWindow bool) *v1alpha1.Postgres {
	return &v1alpha1.Postgres{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: croType.ResourceTypeSpec{
			MaintenanceWindow: maintenanceWindow,
		},
	}
}

func buildTestRedisCR(maintenanceWindow bool) *v1alpha1.Redis {
	return &v1alpha1.Redis{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: croType.ResourceTypeSpec{
			MaintenanceWindow: maintenanceWindow,
		},
	}
}

func Test_VerifyVersionUpgradeNeeded(t *testing.T) {

	type test struct {
		name    string
		current string
		desired string
		wantErr string
		want    bool
	}

	tests := []test{
		{
			name:    "upgrade not needed when versions are the same",
			current: "10.1",
			desired: "10.1",
			want:    false,
		},
		{
			name:    "upgrade not needed when current is higher than desired",
			current: "10.2",
			desired: "10.1",
			want:    false,
		},
		{
			name:    "upgrade needed when current is lower than desired",
			current: "10.1",
			desired: "11.1",
			want:    true,
		},
		{
			name:    "error when current is invalid",
			current: "some broken value",
			desired: "11.1",
			want:    false,
			wantErr: "failed to parse current version: Malformed version: some broken value",
		},
		{
			name:    "error when desired is invalid",
			current: "10.1",
			desired: "some broken value",
			want:    false,
			wantErr: "failed to parse desired version: Malformed version: some broken value",
		},
	}

	for _, tt := range tests {
		got, err := VerifyVersionUpgradeNeeded(tt.current, tt.desired)

		if err != nil {
			if tt.wantErr == "" {
				t.Errorf("VerifyVersionUpgradedNeeded() error: %v", err)
			} else if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Errorf("VerifyVersionUpgradedNeeded() wanted error %v, got error %v", tt.wantErr, err.Error())
			}
		}

		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("VerifyVersionUpgradedNeeded() = %v, want %v", got, tt.want)
		}
	}
}

func Test_VerifyPostgresMaintenanceWindow(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type test struct {
		name    string
		client  client.Client
		want    bool
		wantErr bool
	}

	tests := []test{
		{
			name:    "maintenance window set to false",
			want:    false,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(false)),
			wantErr: false,
		},
		{
			name:    "maintenance window set to true",
			want:    true,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme, buildTestPostgresCR(true)),
			wantErr: false,
		},
		{
			name:    "error getting postgres",
			want:    false,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VerifyPostgresMaintenanceWindow(context.TODO(), tt.client, "test", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyPostgresMaintenanceWindow() error: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifyPostgresMaintenanceWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_VerifyRedisMaintenanceWindow(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}

	type test struct {
		name    string
		client  client.Client
		want    bool
		wantErr bool
	}

	tests := []test{
		{
			name:    "maintenance window set to false",
			want:    false,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(false)),
			wantErr: false,
		},
		{
			name:    "maintenance window set to true",
			want:    true,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme, buildTestRedisCR(true)),
			wantErr: false,
		},
		{
			name:    "error getting redis",
			want:    false,
			client:  moqClient.NewSigsClientMoqWithScheme(scheme),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VerifyRedisMaintenanceWindow(context.TODO(), tt.client, "test", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyRedisMaintenanceWindow() error: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifyRedisMaintenanceWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}
