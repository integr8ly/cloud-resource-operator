## OpenShift CI

### Dockerfile.tools

Base image used on CI for all builds and test jobs. See [here](https://github.com/integr8ly/ci-cd/blob/master/openshift-ci/README.md) for more information on creating and deploying a new image.

#### Build and Test

```
$ docker build -t registry.svc.ci.openshift.org/integr8ly/cro-base-image:latest - < Dockerfile.tools
$ IMAGE_NAME=registry.svc.ci.openshift.org/integr8ly/cro-base-image:latest test/run 
operator-sdk version: "v0.19.0", commit: "8e28aca60994c5cb1aec0251b85f0116cc4c9427", kubernetes version: "v1.18.2", go version: "go1.13.5 linux/amd64"
go version go1.13.5 linux/amd64
go mod tidy
...
SUCCESS!
```

test