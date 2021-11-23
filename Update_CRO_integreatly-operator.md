# Adding Latest CRO to RHMI
Once the CRO release has been cut you will need a PR to the integreatly-operator to
add the latest version of CRO

## Release new version bundle of Cloud Resource Operator

From your CRO branch, navigate to makefile and ensure that:
- PREVIOUS_VERSION and VERSION match the desired versions.
- PREVIOUS_OPERATOR_VERSION contains all the versions that the bundle aims to replace, for example, if you are making a release of version 0.25.0, the PREVIOUS_OPERATOR_VERSIONS
must contain "0.24.0,0.23.0" - where 0.23.0 is the initial bundle version for CRO.
- Run `make gen/csv` which will generate new manifests.
- Ensure that the IMAGE_REG and IMAGE_ORG matches the desired repositories.
- Ensure that the replaces field is present and replaces the previous version.
- If a version skip is required, update the CRO CSV(cloud-resources-operator.clusterserviceversion.yaml) with replaces and skips fields e.g. below yaml replaces 0.24.1 , skips 0.25.0 and 0.26.0 and installs 0.27.0
```yaml
  replaces: cloud-resources.v0.24.1
  skips:
    - cloud-resources.v0.25.0
    - cloud-resources.v0.26.0
  version: 0.27.0
```
- Once the package manifests are ready and merged to master run CRO release pipeline with default params. The pipeline will do the following:
a) Build and push new Cloud Resource Operator image with a tag that matches the VESION field.
b) Build and push new bundle and index based on the PREVIOUS_VERSION and VERSION fields 
- Once the image, bundle and index are pushed, tag the CRO repo.

## Update the CSV in CRO manifest for the Integreatly-operator

To update Integreatly Operator to use the releases bundle of CRO, navigate to [installation file](https://github.com/integr8ly/integreatly-operator/blob/master/products/installation.yaml#L64) 
for the Integreatly Operator and ensure that using the `index` format is selected. 

Update the image of the index to point to the newly created index. For example:

```yaml
 cloud-resources:
    channel: "rhmi"
    installFrom: "index"
    package: "rhmi-cloud-resources"
    index: "quay.io/integreatly/cloud-resource-operator:index-v0.25.0"
```

You need to change the [products.yaml](https://github.com/integr8ly/integreatly-operator/blob/master/products/products.yaml) 
file to point at your new version
```yaml
  - name: cloud-resource-operator
    version: v0.26.0 # replace this with your latest version
    url: "https://github.com/integr8ly/cloud-resource-operator"
    installType: "rhoam/rhmi"
    manifestsDir: "integreatly-cloud-resources"
```

## Update the Manifest

We keep track of bundle changes in the manifest directory in the integreatly-operator repo. These have no effect on the 
logic of integreatly-operator but will need to be correct to pass the `ci/prow/manifests` job. To do this we copy the 
directories the latest version from CRO from`./packagemanifests/<latest-version>` to the 
integreatly-operator directory,
[./manifests/integreatly-cloud-resources](https://github.com/integr8ly/integreatly-operator/tree/master/manifests/integreatly-cloud-resources) 

Also make sure you change the version in the `./manifests/integreatly-cloud-resources/cloud-resources.package.yaml`
file to your latest version of CRO e.g.
```yaml
channels:
- currentCSV: cloud-resources.v0.26.0 # replace this with your version of CRO
  name: rhmi
defaultChannel: rhmi
packageName: rhmi-cloud-resources
```
 
## Update the Vendor, go.mod and go.sum

```bash
go get github.com/integr8ly/cloud-resource-operator
```
This should update the `go.mod` and `go.sum` file with the correct version from master

It should also add the latest version of CRO to the `vendor/` directory and update
`vendor/modules.txt`

## Update CRO version the rhmi_types.go in integreatly-operator

In the integratly-operator go to `apis/v1alpha1/rhmi_types.go` and change the following variables 

```go
VersionCloudResources      ProductVersion = "0.26.0" // replace the version wiht your new version here

OperatorVersionCloudResources      OperatorVersion = "0.26.0" //also replace it here
```

## Verification
- Install RHOAM or RHMI on a byoc cluster.
- Confirm that a new version of CRO is present
- Confirm that Postgres, Redis and blobstorage resources are provisioned correctly.

