# Cloud Resource Operator

[![codecov](https://codecov.io/gh/integr8ly/cloud-resource-operator/branch/master/graph/badge.svg)](https://codecov.io/gh/integr8ly/cloud-resource-operator)

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

## Supported Openshift Versions

Due to a change in how networking is configured for Openshift >= v4.4.6 the use of cro <= v0.16.1 with these Openshift versions is unsupported.
Please use >= v0.17.x of CRO for Openshift >= v4.4.6.


Prerequisites:
- `make`
- [go](https://golang.org/dl/)
- [yq](https://github.com/mikefarah/yq) version v4+
- [operator-sdk](https://github.com/operator-framework/operator-sdk) version v1.14.0.
- [git-secrets](https://github.com/awslabs/git-secrets) - for preventing cloud-provider credentials being included in 
commits
- [OPM](https://docs.openshift.com/container-platform/4.11/cli_reference/opm-cli.html)

Ensure you are running at least `Go 1.18`.
```shell script
$ go version
go version go1.18 darwin/amd64
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

## Snapshots
The cloud resource operator supports the taking of arbitrary snapshots in the AWS provider for both `Postgres` and `Redis`. To take a snapshot you must create a `RedisSnapshot` or `PostgresSnapshot` resource, which should reference the `Redis` or `Postgres` resource you wish to create a snapshot of. The snapshot resource must also exist in the same namespace.
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

## Skip Create
The cloud resource operator continuously reconciles using the strat-config as a source of truth for the current state of the provisioned resources. Should these resources alter from the expected the state the operator will update the resources to match the expected state.  

There can be circumstances where a provisioned resource would need to be altered. If this is the case, add `skipCreate: true` to the resources CR `spec`. This will cause the operator to skip creating or updating the resource. 

## Deployment
The operator expects two configmaps to exist in the namespace it is watching. These configmaps provide the configuration needed to outline the deployment methods and strategies used when provisioning cloud resources.

### Provider configmap
The `cloud-resource-config` configmap defines which provider should be used to provision a specific resource type. Different deployment types can contain different `resource type > provider` mappings.
An example can be seen [here](config/samples/cloud_resource_config.yaml).
For example, a `openshift` deployment type might choose to deploy a Postgres resource type in-cluster (`openshift`), while a `aws` deployment type might choose `AWS` to deploy an RDS instance instead. 

### Strategy configmap
A config map object is expected to exist for each provider (Currently `AWS` or `Openshift`) that will be used by the operator. 
This config map contains information about how to deploy a particular resource type, such as blob storage, with that provider. 
In the Cloud Resources Operator, this provider-specific configuration is called a strategy. An example of an AWS strategy configmap can be seen [here](config/samples/cloud_resources_aws_strategies.yaml).

### Custom Resources
With `Provider` and `Strategy` configmaps in place, cloud resources can be provisioned by creating a custom resource object for the desired resource type. 
An example of a Postgres custom resource can be seen [here](./config/samples/integreatly_v1alpha1_postgres.yaml). 

Each custom resource contains:
- A `secretRef`, containing the name of the secret that will be created by the operator with connection details to the resource
- A `tier`, in this case `production`, which means a production worthy Postgres instance will be deployed 
- A `type`, in this case `openshift`, which will resolve to a cloud provider specified in the `cloud-resource-config` configmap

```yaml
spec:
  # i want my postgres storage information output in a secret named `example-postgres-sec`
  secretRef:
    name: example-postgres-sec
  # i want a postgres storage of a development-level tier
  tier: production
  # i want a postgres storage for the type aws
  type: aws
```

## Resource tagging
Postgres, Redis and Blobstorage resources are tagged with the following key value pairs

```bash
integreatly.org/clusterID: #clusterid
integreatly.org/product-name: #product name
integreatly.org/resource-type: #openshift/aws/gcp 
integreatly.org/resource-name: #postgres/redis/blobstorage
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

Cut a release on Github you need to be an [owner](OWNERS)

- On github ui select on tags 

![tags image](img/tags.png?raw=true)
- Select Releases on then next screen

![release button](img/release-button.png?raw=true)

- On the Release list screen select `Draft a new release` button 

![draft release](img/draft-release.png?raw=true)

- On the Draft release screen add a tag alongside a description that includes the fixes present in the release and select `Publish release`

![release](img/release.png?raw=true)

Update the operator version in the following files:

* Update `VERSION` and `PREV_VERSION` in the [Makefile](Makefile) 

* Generate a new cluster service version:
```sh
make gen/csv
```
* Generate a new bundle and push it to your registry
```sh
make create/olm/bundle
```
* Generate and push new image, bundle and index
```sh
make release/prepare
```

Example:
Starting image for the bundles is 0.23.0, if you are releasing version 0.24.0, ensure that the `PREV_VERSION` is set to `0.23.0`, `VERSION` is set to `0.24.0`

### Deploy with OLM

These steps detail how to deploy CRO through the Operator Lifecycle Manager (OLM) for development purposes.

To deploy a new development release through OLM, we need a bundle, index, and operator container image.
* In order to pull the base image you need to be logged in to [RedHat Container Registry](https://access.redhat.com/RegistryAuthentication).
* The bundle contains manifests and metadata for a single operator version
* The index contains a database of pointers to the operator manifest content and refers to the bundle(s)

The [Makefile](Makefile) automates the creation and tagging of these images, but some of the variables should be adjusted first:
* `VERSION`: this should be set to the value for your development release - in this example we have set it to `10.0.0`
* `IMAGE_ORG`: this should be set to the quay organisation where the images will be pushed
* `UPGRADE`: set to `false` as this version will not replace a previous version

Some of these variables can be passed through at the command line if not set in the [Makefile](Makefile):

```sh
IMAGE_ORG=myorg UPGRADE=false make release/prepare
```

This pushes a new container image for this CRO version (`v10.0.0`), and creates a new [bundle](bundle/) used to create and push a bundle and index container image.

The result is three separate container images in your quay repository:
- quay.io/myorg/cloud-resource-operator:v10.0.0
- quay.io/myorg/cloud-resource-operator:bundle-v10.0.0
- quay.io/myorg/cloud-resource-operator:index-v10.0.0

> **_NOTE_**: To deploy any of the images they must be publicly accessible - ensure that `quay.io/myorg/cloud-resource-operator` is not private

OLM can use this index image to create new operator deployments. A `CatalogSource` is used that references the newly created index tag:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: cro-operator-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/myorg/cloud-resource-operator:index-v10.0.0
```

After a short delay, CRO will be visible from **Operators** -> **OperatorHub** in the Openshift dashboard and can be installed through the GUI.

Alternatively, a `Subscription` object can be created manually (assuming namespace `cloud-resource-operator` exists):

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhmi-cloud-resources
  namespace: cloud-resource-operator
spec:
  channel: rhmi
  name: rhmi-cloud-resources
  source: cro-operator-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
```

### Upgrade with OLM

These steps detail how to deploy the latest version of CRO and trigger an update to your own development version through the Operator Lifecycle Manager (OLM).

To perform a side-by-side upgrade of CRO through OLM, we must create bundle, index and container images for both the version we are upgrading from and the version we are upgrading to.
* The bundle contains manifests and metadata for a single operator version
* The index contains a database of pointers to the operator manifest content and refers to the bundle(s)

First you must checkout the code for the version you would like to upgrade from. For the purposes of this guide we assume that you are upgrading from a tagged previous release. However, if you would like to test an upgrade from the current state of the master branch, follow the first part of the [Deploy with OLM](#deploy-with-olm) guide to create the initial version images. In this example we assume that the latest release is `v0.34.0`.

The [Makefile](Makefile) provides an automated method of creating and pushing the index and bundle images for the latest version of CRO. It uses the latest version number from the [bundles](bundles/) to determine which version is newest. Some of the variables within the [Makefile](Makefile) should also be adjusted:
* `IMAGE_ORG`: this should be set to the quay organisation where the images will be pushed
* `UPGRADE`: set to `false` as this version will not replace a previous version

```sh
IMAGE_ORG=myorg UPGRADE=false make create/olm/bundle
```

This creates a new index and bundle for the original CRO release container image. These are:
- quay.io/integreatly/cloud-resource-operator:v0.34.0
- quay.io/myorg/cloud-resource-operator:bundle-v0.34.0
- quay.io/myorg/cloud-resource-operator:index-v0.34.0

The development release must now be created that we will upgrade to - checkout the code first. For the purposes of this example we assume the development version is `v10.0.0`.

Adjust the [Makefile](Makefile) variables for the new development release:
* `VERSION`: set this to the chosen version - `10.0.0`
* `PREV_VERSION`: this is set to the version we are upgrading from - `0.34.0`
* `IMAGE_ORG`: this should be set to the quay organisation where the images will be pushed

```sh
IMAGE_ORG=myorg make release/prepare
```

This creates a new [bundle](bundles/) for this release, specifying that this version `replaces` the previous version. These manifests are used to generate new index, bundle, and operator container images for the development CRO release:
- quay.io/myorg/cloud-resource-operator:v10.0.0
- quay.io/myorg/cloud-resource-operator:bundle-v10.0.0
- quay.io/myorg/cloud-resource-operator:index-v10.0.0

> **_NOTE_**: To deploy any of the images they must be publicly accessible - ensure that `quay.io/myorg/cloud-resource-operator` is not private

Now that there are bundles, indexes, and operator images for both releases of CRO, the initial version can be deployed. OLM can use the index image to create new operator deployments. A `CatalogSource` is used that references the initial (`v0.34.0`) index tag:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: cro-operator-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/myorg/cloud-resource-operator:index-v0.34.0
```

After a short delay, CRO will be visible from **Operators** -> **OperatorHub** in the Openshift dashboard and can be installed through the GUI.

When ready to trigger the upgrade process, we can update the `CatalogSource` to point to the new index containing the references to both bundles.

```sh
oc edit catalogsource cro-operator-catalog -n openshift-marketplace
```

```diff
apiVersion: operators.coreos.com/v1alpha1
  kind: CatalogSource
  metadata:
    name: cro-operator
    namespace: openshift-marketplace
  spec:
    sourceType: grpc
-   image: quay.io/myorg/cloud-resource-operator:index-v0.34.0
+   image: quay.io/myorg/cloud-resource-operator:index-v10.0.0
```

Navigating to **Installed Operators** -> **Cloud Resource Operator** -> **Subscription** will show a pending upgrade. Click to preview the InstallPlan and approve the update. CRO will be updated from `v0.34.0` to `v10.0.0`.

### Terminology
- `Provider` - A service on which a resource type is provisioned e.g. `aws`, `openshift`
- `Resource type` - Something that can be requested from the operator via a custom resource e.g. `blobstorage`, `redis`
- `Resource` - The result of a resource type created via a provider e.g. `S3 Bucket`, `Azure Blob`
- `Deployment type` - Groups mappings of resource types to providers (see [here](config/samples/cloud_resource_config.yaml)) e.g. `openshift`, `aws`, `gcp`. This provides a layer of abstraction, which allows the end user to not be concerned with _which_ provider is used to deploy the desired resource. 
- `Deployment tier` - Provides a layer of abstraction, which allows the end user to request a resource of a certain level (for example, a `production` worthy Postgres instance), without being concerned with provider-specific deployment details (such as storage capacity, for example). 

### Design
There are a few design philosophies for the Cloud Resource Operator:
- Each resource type (e.g. `BlobStorage`, `Postgres`) should have its own controller
- The end-user should be abstracted from explicitly specifying how the resource is provisioned by default
    - What cloud-provider the resource should be provisioned on should be handled in pre-created config objects
- The end-user should not be abstracted from what provider was used to provision the resource once it's available
    - If a user requests `BlobStorage` they should be made aware it was created on `Amazon AWS`
- Deletion of a custom resource should result in the deletion of the resource in the cloud-provider
