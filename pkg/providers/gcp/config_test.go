package gcp

import (
	"context"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
					mc.GetFunc = func(ctx context.Context, key k8sTypes.NamespacedName, obj runtime.Object) error {
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
