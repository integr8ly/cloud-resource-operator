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

## Running the Cloud Resource Operator (CRO)

## Prerequisites:
- `go`
- `make`
- [git-secrets](https://github.com/awslabs/git-secrets) - for preventing cloud-provider credentials being included in 
commits
- Kubernetes/Openshift 4+ cluster

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
### Run the Operator Locally

Seed your cluster with required resources:
```shell script
$ make cluster/prepare
```

Run the operator:
```shell script
$ make run
```

Clean up your cluster:
```shell script
$ make cluster/clean
```
### Deploy the Operator in cluster

Build and push the image
```shell script
$ make image/push IMAGE_ORG=<your quay org>
```

Update the `deploy/operator.yaml` to point at the newly built image
```
      ...
        - name: cloud-resource-operator
          image: quay.io/<your quay org>/cloud-resource-operator:v0.7.1
          command:
          - cloud-resource-operator
          imagePullPolicy: Always
      ...
```

Seed your cluster with required resources:
```shell script
$ make cluster/prepare
```

Create the service account
```shell script
$ make setup/service_account
```

Deploy the operator
```shell script
$ oc apply -f deploy/operator.yaml
```

### Via the Operator Catalog

*Note* the following steps to installing CRO via the Operator Catalog are intended for Integreatly team members and testing changes in development

Build and push the image to your Quay Org
```shell script
$ make image/push IMAGE_ORG=<your quay org>
```

Push the manifests to your Quay Org
```shell script
$ make manifest/push IMAGE_ORG=<your quay org>
```

Create a new operator source in your Openshift Cluster, updating the `registryNamespace` to your own org name.
```
apiVersion: operators.coreos.com/v1
kind: OperatorSource
metadata:
  name: integreatly-operators
  namespace: openshift-marketplace
spec:
  authorizationToken: {}
  displayName: Integreatly Operators
  endpoint: 'https://quay.io/cnr'
  publisher: Integreatly Publisher
  registryNamespace: <<your quay org name>>
  type: appregistry
```
Once the OLM has reconciled, the CRO should be available in the Catalog.

*Note* Make sure your Quay Repository and Applications are set to public

## Usage
### Seeding a Cluster
After seeding yourcluster with required resources, your cluster is preconfigured to create `managed` and `workshop` resources. Resources deployed in cluster are known as `workshop` and those deployed in AWS are known as `managed`. *Note*, these are arbitrary names and can be updated via the config maps, which are added in the following command : 
```shell script
$ make cluster/prepare
```

To seed workshop resources run the following make target, replacing `<<Resource Type>>` with one of the following `Redis/Postgres/BlobStorage/SMTP`
```shell script
$ make cluster/seed/workshop/<<Resource Type>>
```
To seed managed resources run the following make target, replacing `<<Resource Type>>` with one of the following `Redis/Postgres/BlobStorage/SMTP`
```shell script
$ make cluster/seed/managed/<<Resource Type>>
```

### VPC Peering 
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

### Snapshots
CRO supports the taking of arbitrary snapshots in the AWS provider for both `Postgres` and `Redis`. To take a snapshot you must create a `RedisSnapshot` or `PostgresSnapshot` resource, which should reference the `Redis` or `Postgres` resource you wish to create a snapshot of. The snapshot resource must also exist in the same namespace.
```
apiVersion: integreatly.org/v1alpha1
kind: RedisSnapshot
metadata:
  name: my-redis-snapshot
spec:
  # The redis resource name for the snapshot you want to take
  resourceName: my-redis-resource

```  
*Note* You may experience some downtime in the resource during the creation of the Snapshot

### Skip Create
CRO continuously reconciles using the strat-config as a source of truth for the current state of the provisioned resources. Should these resources alter from the expected the state the operator will update the resources to match the expected state.  

There can be circumstances where a provisioned resource would need to be altered. If this is the case, add `skipCreate: true` to the resources CR `spec`. This will cause the operator to skip creating or updating the resource. 

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

## Resource tagging
Postgres, Redis and Blobstorage resources are tagged with the following key value pairs

```bash
integreatly.org/clusterID: #clusterid
integreatly.org/product-name: #rhmi component product name
integreatly.org/resource-type: #managed/workshop 
integreatly.org/resource-name: #postgres/redis/blobsorage
```

AWS resources can be queried via the aws cli with the cluster id as in the following example
```bash
# clusterid aucunnin-ch5dc
aws resourcegroupstaggingapi get-resources --tag-filters Key=integreatly.org/clusterID,Values=aucunnin-ch5dc | jq
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
Ensure, the latest `CSV` file points to the latest version of the operator image. *Note* the images is referenced twice in the `CSV`. 

Commit changes and open pull request. When the PR is accepted, create a new release tag.

### Terminology
- `Provider` - A service on which a resource type is provisioned e.g. `aws`, `openshift`
- `Resource type` - Something that can be requested from the operator via a custom resource e.g. `blobstorage`, `redis`
- `Resource` - The result of a resource type created via a provider e.g. `S3 Bucket`, `Azure Blob`
- `Deployment type` - Groups mappings of resource types to providers (see [here](deploy/examples/cloud_resource_config.yaml)) e.g. `managed`, `workshop`. This provides a layer of abstraction, which allows the end user to not be concerned with _which_ provider is used to deploy the desired resource. 
- `Deployment tier` - Provides a layer of abstraction, which allows the end user to request a resource of a certain level (for example, a `production` worthy Postgres instance), without being concerned with provider-specific deployment details (such as storage capacity, for example). 

### Design
There are a few design philosophies for CRO:
- Each resource type (e.g. `BlobStorage`, `Postgres`) should have its own controller
- The end-user should be abstracted from explicitly specifying how the resource is provisioned by default
    - What cloud-provider the resource should be provisioned on should be handled in pre-created config objects
- The end-user should not be abstracted from what provider was used to provision the resource once it's available
    - If a user requests `BlobStorage` they should be made aware it was created on `Amazon AWS`
- Deletion of a custom resource should result in the deletion of the resource in the cloud-provider