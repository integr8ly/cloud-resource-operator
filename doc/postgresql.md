# Cloud Resource Operator - PostgreSQL

## Usage

### OpenShift
```
$ make cluster/prepare 
$ make cluster/seed/postgres PROVIDER=openshift
```

### AWS
```
$ make cluster/prepare 
$ make cluster/seed/postgres PROVIDER=aws
```

### GCP
```
$ make cluster/prepare 
$ make cluster/seed/postgres PROVIDER=gcp
```

## Strategy

### AWS
A JSON object containing three keys: `region`, which is the [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region), a `createStrategy`, which is a JSON representation of [this struct](https://docs.aws.amazon.com/sdk-for-go/api/service/rds/#CreateDBInstanceInput), and a `deleteStrategy`, which is a JSON representation of [this struct](https://docs.aws.amazon.com/sdk-for-go/api/service/rds/#DeleteDBInstanceInput).

We currently rely on [AWS to autoscale](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_PIOPS.StorageTypes.html) the `AllocatedStorage`, for this reason CRO does not support modifications to `AllocatedStorage` via the `createStrategy`. If more storage is required, updated `MaxAllocatedStorage` in the `createStrategy`.  

### Openshift
For Kubernetes/Openshift the JSON object contains a single key, `strategy`. The `strategy` object can contain the  following keys, which are used to overwrite specific object configuration: - [PostgresDeploymentSpec](https://godoc.org/k8s.io/api/apps/v1#DeploymentSpec)
- [PostgresServiceSpec](https://godoc.org/k8s.io/api/core/v1#ServiceSpec)
- [PostgresPVCSpec](https://godoc.org/k8s.io/api/core/v1#PersistentVolumeClaimSpec)
- PostgresSecretData - A JSON object with the following keys 
    - `user`
    - `password`
    - `database` 
