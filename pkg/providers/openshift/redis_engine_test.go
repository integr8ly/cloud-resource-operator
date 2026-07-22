package openshift

import (
	"testing"

	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1/types"
)

func TestGetRedisEngineProfile(t *testing.T) {
	tests := []struct {
		name    string
		spec    v1alpha1.RedisSpec
		wantImg string
		wantCmd string
		wantCLI string
		wantDir string
	}{
		{
			name:    "defaults to redis",
			spec:    v1alpha1.RedisSpec{},
			wantImg: defaultRedisImage,
			wantCmd: redisContainerCommand,
			wantCLI: redisCLICommand,
			wantDir: "/var/lib/redis/data",
		},
		{
			name: "valkey defaults to valkey-8 image",
			spec: v1alpha1.RedisSpec{
				Engine: croType.EngineValkey,
			},
			wantImg: defaultValkeyImage,
			wantCmd: valkeyContainerCommand,
			wantCLI: valkeyCLICommand,
			wantDir: "/var/lib/valkey/data",
		},
		{
			name: "valkey 7.2 uses valkey-7 image",
			spec: v1alpha1.RedisSpec{
				Engine:        croType.EngineValkey,
				EngineVersion: "7.2",
			},
			wantImg: valkey7Image,
			wantCmd: valkeyContainerCommand,
			wantCLI: valkeyCLICommand,
			wantDir: "/var/lib/valkey/data",
		},
		{
			name: "valkey 8.0 uses valkey-8 image",
			spec: v1alpha1.RedisSpec{
				Engine:        croType.EngineValkey,
				EngineVersion: "8.0",
			},
			wantImg: defaultValkeyImage,
			wantCmd: valkeyContainerCommand,
			wantCLI: valkeyCLICommand,
			wantDir: "/var/lib/valkey/data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := getRedisEngineProfile(&v1alpha1.Redis{Spec: tt.spec})
			if profile.image != tt.wantImg {
				t.Fatalf("image = %q, want %q", profile.image, tt.wantImg)
			}
			if profile.serverCommand != tt.wantCmd {
				t.Fatalf("serverCommand = %q, want %q", profile.serverCommand, tt.wantCmd)
			}
			if profile.cliCommand != tt.wantCLI {
				t.Fatalf("cliCommand = %q, want %q", profile.cliCommand, tt.wantCLI)
			}
			if profile.dataDir != tt.wantDir {
				t.Fatalf("dataDir = %q, want %q", profile.dataDir, tt.wantDir)
			}
		})
	}
}

func TestReadinessProbeCommand(t *testing.T) {
	if got := readinessProbeCommand(redisCLICommand); got != `redis-cli set liveness-probe "`+"`date`"+`" | grep OK` {
		t.Fatalf("redis probe = %q", got)
	}
	if got := readinessProbeCommand(valkeyCLICommand); got != `valkey-cli set liveness-probe "`+"`date`"+`" | grep OK` {
		t.Fatalf("valkey probe = %q", got)
	}
}
