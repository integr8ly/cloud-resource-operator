package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/utils/pointer"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	configv1 "github.com/openshift/api/config/v1"
	cloudcredentialv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildTestGcpStrategyConfigMap(argsMap map[string]*string) *corev1.ConfigMap {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultConfigMapName,
			Namespace: testNs,
		},
		Data: map[string]string{
			"blobstorage": `{"development": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }}`,
			"redis":       `{"development": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }}`,
			"postgres":    `{"development": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }}`,
			"_network":    `{"development": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "projectID": "", "createStrategy": {}, "deleteStrategy": {} }}`,
		},
	}
	for _, key := range []string{"blobstorage", "redis", "postgres", "_network"} {
		if argsMap[key] != nil {
			configMap.Data[key] = *argsMap[key]
		}
	}
	return &configMap
}

func TestNewConfigMapConfigManager(t *testing.T) {
	type args struct {
		cm        string
		namespace string
		client    client.Client
	}
	tests := []struct {
		name string
		args args
		want *ConfigMapConfigManager
	}{
		{
			name: "placeholder test",
			args: args{},
			want: &ConfigMapConfigManager{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: DefaultConfigMapNamespace,
				client:             nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewConfigMapConfigManager(tt.args.cm, tt.args.namespace, tt.args.client); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewConfigMapConfigManager() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigMapConfigManager_ReadStorageStrategy(t *testing.T) {
	type fields struct {
		configMapName      string
		configMapNamespace string
		client             client.Client
	}
	type args struct {
		ctx  context.Context
		rt   providers.ResourceType
		tier string
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = corev1.AddToScheme(scheme)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *StrategyConfig
		wantErr bool
	}{
		{
			name: "placeholder test",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"development":{"region":"region","projectID":"projectID"}}`),
					}),
				),
			},
			args: args{
				ctx:  context.TODO(),
				rt:   "redis",
				tier: "development",
			},
			want: &StrategyConfig{
				Region:    "region",
				ProjectID: "projectID",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewConfigMapConfigManager(tt.fields.configMapName, tt.fields.configMapNamespace, tt.fields.client)
			got, err := cm.ReadStorageStrategy(tt.args.ctx, tt.args.rt, tt.args.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadStorageStrategy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadStorageStrategy() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultProject(t *testing.T) {
	type args struct {
		ctx context.Context
		c   client.Client
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = configv1.Install(scheme)
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve default project",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
			},
			want:    gcpTestProjectId,
			wantErr: false,
		},
		{
			name: "failed to retrieve default project when undefined",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(map[string]*string{"projectID": aws.String("")}),
				),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "failed to retrieve default project",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDefaultProject(tt.args.ctx, tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDefaultProject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getDefaultProject() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetProjectFromStrategyOrDefault(t *testing.T) {
	type args struct {
		ctx      context.Context
		c        client.Client
		strategy *StrategyConfig
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = configv1.Install(scheme)
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve project from strategy",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
				strategy: &StrategyConfig{
					ProjectID: "projectID-strategy",
				},
			},
			want:    "projectID-strategy",
			wantErr: false,
		},
		{
			name: "successfully retrieve default project",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
				strategy: &StrategyConfig{},
			},
			want:    gcpTestProjectId,
			wantErr: false,
		},
		{
			name: "failed to retrieve project",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				strategy: &StrategyConfig{},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetProjectFromStrategyOrDefault(tt.args.ctx, tt.args.c, tt.args.strategy)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProjectFromStrategyOrDefault() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetProjectFromStrategyOrDefault() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultRegion(t *testing.T) {
	type args struct {
		ctx context.Context
		c   client.Client
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = configv1.Install(scheme)
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve default region",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
			},
			want:    gcpTestRegion,
			wantErr: false,
		},
		{
			name: "failed to retrieve default region when undefined",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(map[string]*string{"region": aws.String("")}),
				),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "failed to retrieve default region",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(scheme)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDefaultRegion(tt.args.ctx, tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDefaultRegion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getDefaultRegion() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRegionFromStrategyOrDefault(t *testing.T) {
	type args struct {
		ctx      context.Context
		c        client.Client
		strategy *StrategyConfig
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = configv1.Install(scheme)
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve region from strategy",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
				strategy: &StrategyConfig{
					Region: "region-strategy",
				},
			},
			want:    "region-strategy",
			wantErr: false,
		},
		{
			name: "successfully retrieve default region",
			args: args{
				c: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpInfrastructure(nil),
				),
				strategy: &StrategyConfig{},
			},
			want:    gcpTestRegion,
			wantErr: false,
		},
		{
			name: "failed to retrieve region",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
				strategy: &StrategyConfig{},
			},
			want:    "",
			wantErr: true,
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

func TestConfigMapConfigManager_getTierStrategyForProvider(t *testing.T) {
	type fields struct {
		configMapName      string
		configMapNamespace string
		client             client.Client
	}
	type args struct {
		ctx  context.Context
		rt   string
		tier string
	}
	scheme := runtime.NewScheme()
	err := cloudcredentialv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	_ = corev1.AddToScheme(scheme)
	_ = configv1.Install(scheme)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *StrategyConfig
		wantErr bool
	}{
		{
			name: "successfully retrieve strategy for provider tier",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"development":{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`),
					}),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want: &StrategyConfig{
				Region:         "region",
				ProjectID:      "projectID",
				CreateStrategy: json.RawMessage(`{}`),
				DeleteStrategy: json.RawMessage(`{}`),
			},
			wantErr: false,
		},
		{
			name: "successfully retrieve strategy for provider tier with default values for project and region",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"development":{"region":"","projectID":"","createStrategy":{},"deleteStrategy":{}}}`),
					}),
					buildTestGcpInfrastructure(nil),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want: &StrategyConfig{
				Region:         gcpTestRegion,
				ProjectID:      gcpTestProjectId,
				CreateStrategy: json.RawMessage(`{}`),
				DeleteStrategy: json.RawMessage(`{}`),
			},
			wantErr: false,
		},
		{
			name: "fail to retrieve default gcp project",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"development":{"region":"region","projectID":"","createStrategy":{},"deleteStrategy":{}}}`),
					}),
					buildTestGcpInfrastructure(map[string]*string{"projectID": pointer.String("")}),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to retrieve default gcp region",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"development":{"region":"","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`),
					}),
					buildTestGcpInfrastructure(map[string]*string{"region": pointer.String("")}),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to retrieve strategy for provider tier when the config map is not found",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("generic error")
					}
					return mc
				}(),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to retrieve strategy for provider tier when resource type is undefined",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      DefaultConfigMapName,
							Namespace: testNs,
						},
						Data: map[string]string{},
					},
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to retrieve strategy for provider tier when unmarshal errors",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{badKey:{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`),
					}),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "fail to retrieve strategy for provider tier when the strategy is not found",
			fields: fields{
				configMapName:      DefaultConfigMapName,
				configMapNamespace: testNs,
				client: moqClient.NewSigsClientMoqWithScheme(scheme,
					buildTestGcpStrategyConfigMap(map[string]*string{
						"redis": aws.String(`{"production":{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`),
					}),
				),
			},
			args: args{
				rt:   "redis",
				tier: "development",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmm := &ConfigMapConfigManager{
				configMapName:      tt.fields.configMapName,
				configMapNamespace: tt.fields.configMapNamespace,
				client:             tt.fields.client,
			}
			got, err := cmm.getTierStrategyForProvider(tt.args.ctx, tt.args.rt, tt.args.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTierStrategyForProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getTierStrategyForProvider() got = %v, want %v", got, tt.want)
			}
		})
	}
}
