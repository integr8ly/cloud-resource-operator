FROM scratch

ARG version
ARG manifest_path=bundles/${version}/manifests
ARG metadata_path=bundles/${version}/metadata

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=rhmi-cloud-resources
LABEL operators.operatorframework.io.bundle.channels.v1=rhmi
LABEL operators.operatorframework.io.bundle.channel.default.v1=rhmi
LABEL operators.operatorframework.io/builder=operator-sdk-v1.13.0+git
LABEL operators.operatorframework.io/project_layout=go.kubebuilder.io/v2

# Copy files to locations specified by labels.
COPY ${manifest_path} /manifests/
COPY ${metadata_path} /metadata/
