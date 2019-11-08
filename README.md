# Cloud Resource Operator

[![Coverage Status](https://coveralls.io/repos/github/integr8ly/cloud-resource-operator/badge.svg)](https://coveralls.io/github/integr8ly/cloud-resource-operator)

Operator to provision resources such as Postgres, Redis and storage for you, either in-cluster or through a cloud
provider such as Amazon AWS.

This operator depends on the [Cloud Credential Operator](https://github.com/openshift/cloud-credential-operator) for
creating certain resources such as Amazon AWS Credentials. If using the AWS provider, ensure the Cloud Credential
Operator is running.

***Note: This operator is in the very early stages of development. There will be bugs and regular breaking changes***

## Supported Cloud Resources
| Cloud Resource 	| Openshift 	| AWS 	|
|:--------------:	|:---------:	|:---------:	|
|  [Blob Storage](./doc/blobstorage.md)  	|     :x:     	| :heavy_check_mark: 	|
|     [Redis](./doc/redis.md)  	|     :heavy_check_mark:     	|  :heavy_check_mark: 	|
|   [PostgreSQL](./doc/postgresql.md) 	|     :heavy_check_mark:     	|  :heavy_check_mark:  	|
|      [SMTP](./doc/smtp.md)     	|     :x:     	|  :heavy_check_mark:  	|

## Running the Cloud Resource Operator
## Locally

Prerequisites:
- `go`
- `make`
- [git-secrets](https://github.com/awslabs/git-secrets) - for preventing cloud-provider credentials being included in 
commits

Ensure you are running at least `Go 1.13`.
```shell script
$ go version
go version go1.13 darwin/amd64
```

If not, ensure Go Modules are enabled.

Clone this repository into your working directory, outside of `$GOPATH`. For example:
```shell script
$ cd ~/dev
$ git clone git@github.com:integr8ly/cloud-resource-operator.git
```

Seed the Kubernetes/OpenShift cluster with required resources:
```shell script
$ make cluster/prepare
```

Run the operator:
```shell script
$ make run
```

Clean up the Kubernetes/OpenShift cluster:
```shell script
$ make cluster/clean
```

## VPC Peering 
Currently AWS resources are deployed into a separate Virtual Private Cloud (VPC) than the VPC the cluster is deployed into. In order for these to communicate, a `peering connection` must be established between the two VPCS. To do this:
- In the AWS VPC console, create a new peering connection between the two VPCs. This is a two-way communication channel, so only one needs to be created
- Select the newly created connection, then click `actions > accept request` to accept the peering request
- Edit the cluster VPC route table. Create a new route that contains the resource VPC's CIDR block as the `destination` and the newly created peering connection as the `target`
- Edit the resource VPC's route table. Create a new route that contains the CIDR block of the cluster VPC as the `destination` and the peering connection as the `target`. 

The two VPCs should now be able to communicate with each other. 

## Via the Operator Catalog

***In development***

## Deployment
Two config maps are expected by the operator, which will provide configuration needed to outline the deployment methods and strategies available to the Cloud Resources.

### Provider
A config map object is expected to exist with a mapping from type name to deployment method, an example of this can be seen [here](deploy/examples/cloud_resource_config.yaml).

### Strategy 
A config map object is expected to exist for each provider (`AWS` or `Openshift`) that will be used by the operator. This config map contains provider-specific information about how to deploy a particular resource type, such as blob storage. In the Cloud Resources Operator, this provider-specific configuration is called a strategy. An example of the AWS strategy can be seen [here](deploy/examples/cloud_resources_aws_strategies.yaml).

### Custom Resources
With `Provider` and `Strategy` configmaps in place, resources can be created through a custom resource. An example of a Blob Storage CR can be seen [here](./deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml). 
In the spec of this CR, we outline the secret name where we want the blob storage information to be output. If the provider type were AWS, for example, the output secret would contain connection information to the S3 bucket that was created. The `tier` outlines the `Strategy` we wish to use. The `type` references the deployment to be used.
```
spec:
  # i want my blob storage information output in a Secret named blob-test in the current namespace
  secretRef:
    name: blob-test
  # i want a blob storage of a development-level tier
  tier: development
  # i want a blob storage for the type managed
  type: managed

```
## Development

### Contributing

### Testing
To run e2e tests from a built image:
```
$ make test/e2e/image IMAGE=<<built image>>
```
To run e2e tests locally:
```
$ make test/e2e/local
```
To run unit tests:
```
$ make test/unit
```

- Write tests
- Implement changes
- Run code fixer, `make code/fix`
- Run tests, `make test/unit`
- Make a PR

### Terminology
- `Resource type` - Something that can be requested from the operator via a custom resource e.g. `blobstorage`, `redis`
- `Provider` - A service on which a resource type is provisioned e.g. `aws`, `openshift`
- `Resource` - The result of a resource type created via a provider e.g. `S3 Bucket`, `Azure Blob`

### Design
There are a few design philosophies for the Cloud Resource Operator:
- Each resource type (e.g. `BlobStorage`, `Postgres`) should have its own controller
- The end-user should be abstracted from explicitly specifying how the resource is provisioned by default
    - What cloud-provider the resource should be provisioned on should be handled in pre-created config objects
- The end-user should not be abstracted from what provider was used to provision the resource once it's available
    - If a user requests `BlobStorage` they should be made aware it was created on `Amazon AWS`
- Deletion of a custom resource should result in the deletion of the resource in the cloud-provider