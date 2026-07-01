package types

import "testing"

func TestIsSupportedCacheEngine(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		want   bool
	}{
		{name: "empty engine is valid default", engine: "", want: true},
		{name: "redis", engine: EngineRedis, want: true},
		{name: "valkey", engine: EngineValkey, want: true},
		{name: "unsupported", engine: "memcached", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedCacheEngine(tt.engine); got != tt.want {
				t.Fatalf("IsSupportedCacheEngine(%q) = %v, want %v", tt.engine, got, tt.want)
			}
		})
	}
}
