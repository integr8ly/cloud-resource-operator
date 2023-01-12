package resources

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	defaultSecurityGroupPostfix = "security-group"
	defaultIdentifierLength     = 40
)

func buildTestRedisSnapshotCR() *v1alpha1.RedisSnapshot {
	return &v1alpha1.RedisSnapshot{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Status: croType.ResourceTypeSnapshotStatus{
			SnapshotID: "test-identifier",
		},
	}
}

func newFakeAwsInfrastructure() *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name: "cluster",
		},
		Status: configv1.InfrastructureStatus{
			InfrastructureName: "test",
			PlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
				AWS: &configv1.AWSPlatformStatus{
					Region: "test-region",
				},
			},
		},
	}
}

func TestGetForcedReconcileTimeOrDefault(t *testing.T) {
	type args struct {
		defaultTo time.Duration
	}
	var tests = []struct {
		name                  string
		want                  time.Duration
		envForceReconcileTime string
		args                  args
	}{
		{
			name: "test function returns default",
			args: args{
				defaultTo: time.Second * 60,
			},
			want: time.Second * 60,
		},
		{
			name: "test accepts env var and returns value",
			args: args{
				defaultTo: time.Second * 60,
			},
			envForceReconcileTime: "30",
			want:                  time.Nanosecond * 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envForceReconcileTime != "" {
				if err := os.Setenv(EnvForceReconcileTimeout, tt.envForceReconcileTime); err != nil {
					t.Errorf("GetReconcileTime() err = %v", err)
				}
			}
			if got := GetForcedReconcileTimeOrDefault(tt.args.defaultTo); got != tt.want {
				t.Errorf("GetReconcileTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetOrganizationTag(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "test env var does not exist return default",
			want: DefaultTagKeyPrefix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetOrganizationTag(); got != tt.want {
				t.Errorf("GetOrganizationTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildInfraName(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	err := configv1.Install(fakeScheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx     context.Context
		c       client.Client
		postfix string
		n       int
	}
	tests := []struct {
		name     string
		args     args
		wantErr  bool
		expected string
	}{
		{
			name: "successfully return an id used for infra resources",
			args: args{
				ctx:     context.TODO(),
				c:       fake.NewFakeClientWithScheme(fakeScheme, newFakeAwsInfrastructure()),
				postfix: defaultSecurityGroupPostfix,
				n:       defaultIdentifierLength,
			},
			wantErr:  false,
			expected: "testsecuritygroup",
		},
		{
			name: "error getting cluster id",
			args: args{
				ctx:     context.TODO(),
				c:       fake.NewFakeClientWithScheme(fakeScheme),
				postfix: defaultSecurityGroupPostfix,
				n:       defaultIdentifierLength,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildInfraName(tt.args.ctx, tt.args.c, tt.args.postfix, tt.args.n)
			if tt.wantErr {
				if err != nil {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("expected %s to equal %s", got, tt.expected)
			}
		})
	}
}

func TestBuildTimestampedInfraNameFromObjectCreation(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	err := configv1.Install(fakeScheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx context.Context
		c   client.Client
		om  controllerruntime.ObjectMeta
		n   int
	}
	tests := []struct {
		name     string
		args     args
		wantErr  bool
		expected string
	}{
		{
			name: "successfully return a timestamped infra name from object creation",
			args: args{
				ctx: context.TODO(),
				c:   fake.NewFakeClientWithScheme(fakeScheme, newFakeAwsInfrastructure()),
				om:  buildTestRedisSnapshotCR().ObjectMeta,
				n:   defaultIdentifierLength,
			},
			wantErr:  false,
			expected: "testtesttest000101010000000000UTC",
		},
		{
			name: "error getting cluster id",
			args: args{
				ctx: context.TODO(),
				c:   fake.NewFakeClientWithScheme(fakeScheme),
				om:  buildTestRedisSnapshotCR().ObjectMeta,
				n:   defaultIdentifierLength,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildTimestampedInfraNameFromObjectCreation(tt.args.ctx, tt.args.c, tt.args.om, tt.args.n)
			if tt.wantErr {
				if err != nil {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("expected %s to equal %s", got, tt.expected)
			}
		})
	}
}

func TestBuildTimestampedInfraNameFromObject(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	err := configv1.Install(fakeScheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx context.Context
		c   client.Client
		om  controllerruntime.ObjectMeta
		n   int
	}
	tests := []struct {
		name     string
		args     args
		wantErr  bool
		expected string
	}{
		{
			name: "successfully return a timestamped infra name from object",
			args: args{
				ctx: context.TODO(),
				c:   fake.NewFakeClientWithScheme(fakeScheme, newFakeAwsInfrastructure()),
				om:  buildTestRedisSnapshotCR().ObjectMeta,
				n:   defaultIdentifierLength,
			},
			wantErr:  false,
			expected: "testtesttest1652356208",
		},
		{
			name: "error getting cluster id",
			args: args{
				ctx: context.TODO(),
				c:   fake.NewFakeClientWithScheme(fakeScheme),
				om:  buildTestRedisSnapshotCR().ObjectMeta,
				n:   defaultIdentifierLength,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildTimestampedInfraNameFromObject(tt.args.ctx, tt.args.c, tt.args.om, tt.args.n)
			if tt.wantErr {
				if err != nil {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got, "testtesttest") {
				t.Fatalf("expected %s to contain testtesttest", got)
			}
		})
	}
}
