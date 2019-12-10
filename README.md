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
Currently AWS resources are deployed into a separate Virtual Private Cloud (VPC) than the VPC that the cluster is deployed into. In order for these to communicate, a `peering connection` must be established between the two VPCS. To do this:
1. Create a new peering connection between the two VPCs.
  - Go to `VPC` > `Peering Connections`
  - Click on `Create Peering Connection`
    - **NOTE**: This is a two-way communication channel so only one needs to be created.
  - Select the newly created connection, then click `Actions` > `Accept Request` to accept the peering request.
2. Edit your cluster's VPC route table
  - Go to `VPC` > `Your VPCs`
  - Find the VPC your cluster is using and click on its route table under the heading `Main Route Table`
    - Your cluster VPC name is usually in a format of `<cluster-name>-<uid>-vpc`
  - Select the cluster route table and click on `Action` > `Edit routes`
  - Add a new route with the following details:
    ```
    Destination: <resource VPC CIDR block>
    Target: <newly created peering connection>
    ```
  - Click on `Save routes`
3. Edit the resource VPC route table
  - Go to `VPC` > `Your VPCs`
  - Find the VPC where the AWS resources are provisioned in and click on its route table under the heading `Main Route Table`
    - The name of this VPC is usually empty or named as `default`
  - Select the cluster route table and click on `Action` > `Edit routes`
  - Add a new route with the following details:
    ```
    Destination: <your cluster's VPC CIDR block>
    Target: <newly created peering connection>
    ```
  - Click on `Save routes`
4. Edit the Security Groups associated with the resource VPC to ensure database and cache traffic can pass between the two VPCs.

The two VPCs should now be able to communicate with each other. 

## Via the Operator Catalog

***In development***

## Deployment
The operator expects two configmaps to exist in the namespace it is watching. These configmaps provide the configuration needed to outline the deployment methods and strategies used when provisioning cloud resources.

### Provider configmap
The `cloud-resource-config` configmap defines which provider should be used to provision a specific resource type. Different deployment types can contain different `resource type > provider` mappings.
An example can be seen [here](deploy/examples/cloud_resource_config.yaml).
For example, a `workshop` deployment type might choose to deploy a Postgres resource type in-cluster (`openshift`), while a `managed` deployment type might choose `AWS` to deploy an RDS instance instead. 

### Strategy configmap
A config map object is expected to exist for each provider (Currently `AWS` or `Openshift`) that will be used by the operator. 
This config map contains information about how to deploy a particular resource type, such as blob storage, with that provider. 
In the Cloud Resources Operator, this provider-specific configuration is called a strategy. An example of an AWS strategy configmap can be seen [here](deploy/examples/cloud_resources_aws_strategies.yaml).

### Custom Resources
With `Provider` and `Strategy` configmaps in place, cloud resources can be provisioned by creating a custom resource object for the desired resource type. 
An example of a Postgres custom resource can be seen [here](./deploy/crds/integreatly_v1alpha1_postgres_cr.yaml). 

Each custom resource contains:
- A `secretRef`, containing the name of the secret that will be created by the operator with connection details to the resource
- A `tier`, in this case `production`, which means a production worthy Postgres instance will be deployed 
- A `type`, in this case `managed`, which will resolve to a cloud provider specified in the `cloud-resource-config` configmap

```yaml
spec:
  # i want my postgres storage information output in a secret named `example-postgres-sec`
  secretRef:
    name: example-postgres-sec
  # i want a postgres storage of a development-level tier
  tier: production
  # i want a postgres storage for the type managed
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


### Releasing


Update the operator version in the following files:

* Update [version/version.go](version/version.go) (`Version = "<version>"`)

* Update `VERSION` and `PREV_VERSION` (the previous version) in the [Makefile](Makefile) 

* Generate a new cluster service version:
```sh
make gen/csv
```

Commit changes and open pull request. When the PR is accepted, create a new release tag.

### Terminology
- `Provider` - A service on which a resource type is provisioned e.g. `aws`, `openshift`
- `Resource type` - Something that can be requested from the operator via a custom resource e.g. `blobstorage`, `redis`
- `Resource` - The result of a resource type created via a provider e.g. `S3 Bucket`, `Azure Blob`
- `Deployment type` - Groups mappings of resource types to providers (see [here](deploy/examples/cloud_resource_config.yaml)) e.g. `managed`, `workshop`. This provides a layer of abstraction, which allows the end user to not be concerned with _which_ provider is used to deploy the desired resource. 
- `Deployment tier` - Provides a layer of abstraction, which allows the end user to request a resource of a certain level (for example, a `production` worthy Postgres instance), without being concerned with provider-specific deployment details (such as storage capacity, for example). 

### Design
There are a few design philosophies for the Cloud Resource Operator:
- Each resource type (e.g. `BlobStorage`, `Postgres`) should have its own controller
- The end-user should be abstracted from explicitly specifying how the resource is provisioned by default
    - What cloud-provider the resource should be provisioned on should be handled in pre-created config objects
- The end-user should not be abstracted from what provider was used to provision the resource once it's available
    - If a user requests `BlobStorage` they should be made aware it was created on `Amazon AWS`
- Deletion of a custom resource should result in the deletion of the resource in the cloud-provider