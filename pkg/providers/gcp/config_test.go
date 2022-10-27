package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

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
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{"region":"region","projectID":"projectID"}}`}
						}
						return nil
					}
					return mc
				}(),
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
			cm := NewDefaultConfigManager(tt.fields.client)
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
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve default project",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
			},
			want:    "projectID",
			wantErr: false,
		},
		{
			name: "failed to retrieve default project when undefined",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "failed to retrieve default project",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve project from strategy",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
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
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									ProjectID: "projectID",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				strategy: &StrategyConfig{},
			},
			want:    "projectID",
			wantErr: false,
		},
		{
			name: "failed to retrieve project",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve default region",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									Region: "region",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
			},
			want:    "region",
			wantErr: false,
		},
		{
			name: "failed to retrieve default region when undefined",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									Region: "",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "failed to retrieve default region",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "successfully retrieve region from strategy",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									Region: "region",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
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
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *v1.Infrastructure:
							cr.Status.PlatformStatus = &v1.PlatformStatus{
								GCP: &v1.GCPPlatformStatus{
									Region: "region",
								},
								Type: v1.GCPPlatformType,
							}
						}
						return nil
					}
					return mc
				}(),
				strategy: &StrategyConfig{},
			},
			want:    "region",
			wantErr: false,
		},
		{
			name: "failed to retrieve region",
			args: args{
				c: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
				configMapName:      testName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"development":{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`}
						}
						return nil
					}
					return mc
				}(),
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
			name: "fail to retrieve strategy for provider tier when the config map is not found",
			fields: fields{
				configMapName:      testName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
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
				configMapName:      testName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *corev1.ConfigMap:
							cr.Data = nil
						}
						return nil
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
			name: "fail to retrieve strategy for provider tier when unmarshal errors",
			fields: fields{
				configMapName:      testName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{badKey:{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`}
						}
						return nil
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
			name: "fail to retrieve strategy for provider tier when the strategy is not found",
			fields: fields{
				configMapName:      testName,
				configMapNamespace: testNs,
				client: func() client.Client {
					mc := moqClient.NewSigsClientMoqWithScheme(nil)
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj client.Object) error {
						switch cr := obj.(type) {
						case *corev1.ConfigMap:
							cr.Data = map[string]string{"redis": `{"production":{"region":"region","projectID":"projectID","createStrategy":{},"deleteStrategy":{}}}`}
						}
						return nil
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
