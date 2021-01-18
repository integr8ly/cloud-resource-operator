package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"testing"

	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	configv1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/config/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var configMapNameSpace, _ = k8sutil.GetWatchNamespace()

func newFakeInfrastructure() *configv1.Infrastructure {
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

func TestNewConfigManager(t *testing.T) {
	cases := []struct {
		name              string
		cmName            string
		expectedName      string
		cmNamespace       string
		expectedNamespace string
		client            client.Client
	}{
		{
			name:              "test defaults are set when empty strings are provided",
			cmName:            "",
			cmNamespace:       "",
			expectedName:      "cloud-resources-aws-strategies",
			expectedNamespace: configMapNameSpace,
			client:            nil,
		},
		{
			name:              "test defaults are not used when non-empty strings are provided",
			cmName:            "test",
			cmNamespace:       "test",
			expectedName:      "test",
			expectedNamespace: "test",
			client:            nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewConfigMapConfigManager(tc.cmName, tc.cmNamespace, tc.client)
			if cm.configMapName != tc.expectedName {
				t.Fatalf("unexpected name, expected %s but got %s", tc.expectedName, cm.configMapName)
			}
			if cm.configMapNamespace != tc.expectedNamespace {
				t.Fatalf("unexpected namespace, expected %s but got %s", tc.expectedNamespace, cm.configMapNamespace)
			}
		})
	}
}

func TestConfigManager_ReadBlobStorageStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	sc := &StrategyConfig{
		Region:         "eu-west-1",
		CreateStrategy: json.RawMessage("{\"bucket\":\"testbucket\"}"),
	}
	rawStratCfg, err := json.Marshal(sc)
	if err != nil {
		t.Fatal("failed to marshal strategy config", err)
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme, &v1.ConfigMap{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Data: map[string]string{
			"blobstorage": fmt.Sprintf("{\"test\": %s}", string(rawStratCfg)),
		},
	})
	cases := []struct {
		name                string
		cmName              string
		cmNamespace         string
		tier                string
		expectedRegion      string
		expectedRawStrategy string
		client              client.Client
		expectErr           bool
	}{
		{
			name:                "test strategy is parsed successfully when tier exists",
			cmName:              "test",
			cmNamespace:         "test",
			tier:                "test",
			expectedRegion:      "eu-west-1",
			expectedRawStrategy: string(sc.CreateStrategy),
			client:              fakeClient,
		},
		{
			name:        "test error is returned when strategy does not exist for tier",
			cmName:      "test",
			cmNamespace: "test",
			tier:        "doesnotexist",
			expectErr:   true,
			client:      fakeClient,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := NewConfigMapConfigManager(tc.cmName, tc.cmNamespace, tc.client)
			sc, err := cm.ReadStorageStrategy(context.TODO(), providers.BlobStorageResourceType, tc.tier)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatal("unexpected error", err)
			}
			if sc.Region != tc.expectedRegion {
				t.Fatalf("unexpected region, expected %s but got %s", tc.expectedRegion, sc.Region)
			}
			if string(sc.CreateStrategy) != tc.expectedRawStrategy {
				t.Fatalf("unexpected raw strategy, expected %s but got %s", tc.expectedRawStrategy, sc.CreateStrategy)
			}
		})
	}
}

func TestGetRegionFromStrategyOrDefault(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	v1.AddToScheme(fakeScheme)
	configv1.SchemeBuilder.AddToScheme(fakeScheme)

	fakeStrategy := &StrategyConfig{
		Region: "strategy-region",
	}
	fakeInfra := newFakeInfrastructure()

	type args struct {
		ctx      context.Context
		c        client.Client
		strategy *StrategyConfig
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "fail to get default region",
			args: args{
				ctx:      context.TODO(),
				c:        fake.NewFakeClientWithScheme(fakeScheme),
				strategy: fakeStrategy,
			},
			wantErr: true,
		},
		{
			name: "strategy defines region",
			args: args{
				ctx:      context.TODO(),
				c:        fake.NewFakeClientWithScheme(fakeScheme, fakeInfra),
				strategy: fakeStrategy,
			},
			want: fakeStrategy.Region,
		},
		{
			name: "default used when strategy does not define region",
			args: args{
				ctx: context.TODO(),
				c:   fake.NewFakeClientWithScheme(fakeScheme, fakeInfra),
				strategy: &StrategyConfig{
					Region: "",
				},
			},
			want: fakeInfra.Status.PlatformStatus.AWS.Region,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetRegionFromStrategyOrDefault(tt.args.ctx, tt.args.c, tt.args.strategy)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRegionFromStrategyOrDefault() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetRegionFromStrategyOrDefault() got = %v, want %v", got, tt.want)
			}
		})
	}
}
