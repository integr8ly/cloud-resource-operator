package gcp

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
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
			name:    "placeholder test",
			fields:  fields{},
			args:    args{},
			want:    nil,
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
