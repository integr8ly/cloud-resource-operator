package openshift

import (
	"fmt"
	"strings"

	"github.com/integr8ly/cloud-resource-operator/api/integreatly/v1alpha1"
)

const (
	defaultRedisImage = "registry.redhat.io/rhel9/redis-7"
	// defaultValkeyImage is used when engine is valkey and engineVersion is empty or 8.x.
	// Red Hat currently publishes rhel9/valkey-8 for OpenShift; valkey-7 is only used when
	// engineVersion explicitly starts with "7" (e.g. manual migration).
	defaultValkeyImage = "registry.redhat.io/rhel9/valkey-8"
	valkey7Image       = "registry.redhat.io/rhel9/valkey-7"

	valkeyConfigVolumeName = "valkey-config"
	valkeyConfigMapName    = "valkey-config"
	valkeyConfigMapKey     = "valkey.conf"
	valkeyContainerName    = "valkey"
	valkeyContainerCommand = "/usr/bin/valkey-server"
	redisCLICommand        = "redis-cli"
	valkeyCLICommand       = "valkey-cli"
)

type redisEngineProfile struct {
	engineName       string
	containerName    string
	image            string
	serverCommand    string
	cliCommand       string
	configFileName   string
	configMountDir   string
	configArgPath    string
	dataDir          string
	configVolumeName string
	configMapName    string
}

func getRedisEngineProfile(r *v1alpha1.Redis) redisEngineProfile {
	if r.IsValkey() {
		image := defaultValkeyImage
		if strings.HasPrefix(r.GetEngineVersion(), "7") {
			image = valkey7Image
		}
		return redisEngineProfile{
			engineName:       r.EngineDisplayName(),
			containerName:    valkeyContainerName,
			image:            image,
			serverCommand:    valkeyContainerCommand,
			cliCommand:       valkeyCLICommand,
			configFileName:   valkeyConfigMapKey,
			configMountDir:   "/etc/valkey.d",
			configArgPath:    "/etc/valkey.d/valkey.conf",
			dataDir:          "/var/lib/valkey/data",
			configVolumeName: valkeyConfigVolumeName,
			configMapName:    valkeyConfigMapName,
		}
	}

	return redisEngineProfile{
		engineName:       r.EngineDisplayName(),
		containerName:    redisContainerName,
		image:            defaultRedisImage,
		serverCommand:    redisContainerCommand,
		cliCommand:       redisCLICommand,
		configFileName:   redisConfigMapKey,
		configMountDir:   "/etc/redis.d",
		configArgPath:    "/etc/redis.d/redis.conf",
		dataDir:          "/var/lib/redis/data",
		configVolumeName: redisConfigVolumeName,
		configMapName:    redisConfigMapName,
	}
}

func getRedisConfData(dataDir string) string {
	return fmt.Sprintf(`protected-mode no
port 6379
timeout 0
tcp-keepalive 300
daemonize no
supervised no
loglevel notice
databases 16
save 900 1
save 300 10
save 60 10000
stop-writes-on-bgsave-error yes
rdbcompression yes
rdbchecksum yes
dbfilename dump.rdb
slave-serve-stale-data yes
slave-read-only yes
repl-diskless-sync no
repl-disable-tcp-nodelay no
appendfilename "appendonly.aof"
appendfsync everysec
no-appendfsync-on-rewrite no
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb
aof-load-truncated yes
lua-time-limit 5000
activerehashing no
aof-rewrite-incremental-fsync yes
dir %s
`, dataDir)
}

func readinessProbeCommand(cliCommand string) string {
	return fmt.Sprintf("%s set liveness-probe \"`date`\" | grep OK", cliCommand)
}
