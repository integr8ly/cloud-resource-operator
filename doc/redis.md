# Cloud Resource Operator - Redis

## Usage

### OpenShift
```
$ make cluster/prepare 
$ make cluster/seed/redis PROVIDER=openshift
```

### AWS
```
$ make cluster/prepare 
$ make cluster/seed/redis PROVIDER=aws
```

### GCP
```
$ make cluster/prepare 
$ make cluster/seed/redis PROVIDER=gcp
```

## Strategy

### AWS
A JSON object containing two keys: `region`, which is the [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region), and `createStrategy`, which is a JSON representation of [this struct](https://docs.aws.amazon.com/sdk-for-go/api/service/elasticache/#CreateReplicationGroupInput)

### Openshift
For Kubernetes/Openshift the JSON object contains a single key, `strategy`. The `strategy` object can contain the  following keys, which are used to overwrite specific object configuration: 
- [RedisDeploymentSpec](https://godoc.org/k8s.io/api/apps/v1#DeploymentSpec)
- [RedisServiceSpec](https://godoc.org/k8s.io/api/core/v1#ServiceSpec)
- [RedisPVCSpec](https://godoc.org/k8s.io/api/core/v1#PersistentVolumeClaimSpec)
- RedisConfigMapData - A `map[string]string` with the key `redis.conf` 
