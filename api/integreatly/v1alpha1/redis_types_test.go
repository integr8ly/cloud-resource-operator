package v1alpha1

import (
	"testing"

	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
)

func TestRedisGetEngine(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		want   string
	}{
		{name: "defaults to redis", engine: "", want: croType.EngineRedis},
		{name: "explicit redis", engine: croType.EngineRedis, want: croType.EngineRedis},
		{name: "explicit valkey", engine: croType.EngineValkey, want: croType.EngineValkey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Redis{Spec: RedisSpec{Engine: tt.engine}}
			if got := r.GetEngine(); got != tt.want {
				t.Fatalf("GetEngine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedisIsValkey(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		want   bool
	}{
		{name: "empty engine is not valkey", engine: "", want: false},
		{name: "redis is not valkey", engine: croType.EngineRedis, want: false},
		{name: "valkey is valkey", engine: croType.EngineValkey, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Redis{Spec: RedisSpec{Engine: tt.engine}}
			if got := r.IsValkey(); got != tt.want {
				t.Fatalf("IsValkey() = %v, want %v", got, tt.want)
			}
		})
	}
}
