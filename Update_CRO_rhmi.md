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
- Create new CRO image by running `make image/build/push` or point to the existing one in the CSV you have created.
- Ensure that the replaces field is present and replaces previous version.
- Run `make create/olm/bundle` - this will create and push bundles and indices to given repository, as well as validating them.

## Update the CSV in CRO manifest for the Integreatly-operator

To update Integreatly Operator to use the releases bundle of CRO, navigate to [installation file](https://github.com/integr8ly/integreatly-operator/blob/master/products/installation.yaml#L64) for the Integreatly Operator and ensure that using the `index` format is selected. 

Update the image of the index to point to the newly created index. For example:

```
 cloud-resources:
    channel: "rhmi"
    installFrom: "index"
    package: "rhmi-cloud-resources"
    index: "quay.io/integreatly/cloud-resource-operator:index-v0.25.0"
```

## Verification
- Install RHOAM or RHMI on a byoc cluster.
- Confirm that a new version of CRO is present
- Confirm that Postgres, Redis and blobstorage resources are provisioned correctly.

