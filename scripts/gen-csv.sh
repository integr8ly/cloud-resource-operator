#!/usr/bin/env bash
# Used to generate new bundle and index files for a new
# release as well as updating base files used to generate from
set -e
set -o pipefail

IMAGE_ORG="${IMAGE_ORG:-integreatly}"
IMAGE_REG="${IMAGE_REG:-quay.io}"
IMAGE_NAME="${IMAGE_NAME:-cloud-resource-operator}"
KUSTOMIZE="${KUSTOMIZE:-"/usr/local/bin/kustomize"}"
CHANNEL="${CHANNEL:-rhmi}"
PREV_VERSION="${PREV_VERSION}"

# Set sed -i as it's different for mac vs gnu
if [[ $(uname) = Darwin ]]; then
  SED_INLINE=(sed -i '')
else
  SED_INLINE=(sed -i)
fi

if [[ -z "$SEMVER" ]]; then
 echo "ERROR: no SEMVER value set"
 exit 1
fi

if [[ $SEMVER =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*)?(\+[0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*)?$ ]]; then
  echo "Valid version string: ${SEMVER}"
else
  echo "Error: Invalid version string: ${SEMVER}"
  exit 1
fi

VERSION=$(echo "$SEMVER" | awk -F - '{print $1}')
OPERATOR_IMG=$IMAGE_REG/$IMAGE_ORG/$IMAGE_NAME:v$VERSION


start() {
  update_base_csv
  create_bundle
  update_bundle
  create_index
}

update_base_csv() {
  BASE_CSV=config/manifests/bases/$IMAGE_NAME.clusterserviceversion.yaml
  yq e -i ".metadata.name=\"$IMAGE_NAME.v$VERSION\"" $BASE_CSV
  yq e -i ".spec.version=\"$VERSION\"" $BASE_CSV
  yq e -i ".metadata.annotations.containerImage=\"$OPERATOR_IMG\"" $BASE_CSV
  if [[ "${VERSION}" != "${PREV_VERSION}" ]]; then
    yq e -i ".spec.replaces=\"$IMAGE_NAME.v$PREV_VERSION\"" $BASE_CSV
  fi
}

create_bundle() {
  "${KUSTOMIZE[@]}" build config/manifests \
    | operator-sdk generate bundle \
      --kustomize-dir config/manifests \
      --output-dir bundles/$VERSION \
      --version $VERSION \
      --default-channel $CHANNEL \
      --package cloud-resource-operator \
      --channels $CHANNEL
  rm bundle.Dockerfile
}

update_bundle() {
  BUNDLE_CSV=bundles/$VERSION/manifests/$IMAGE_NAME.clusterserviceversion.yaml
  "${SED_INLINE[@]}" "s/Version = \"${PREV_VERSION}\"/Version = \"${VERSION}\"/g" version/version.go
  yq e -i ".channels[0].currentCSV=\"$IMAGE_NAME.v$VERSION\"" bundles/$IMAGE_NAME.package.yaml
  yq e -i ".metadata.name=\"cloud-resources.v$VERSION\"" $BUNDLE_CSV
  yq e -i ".spec.replaces=\"cloud-resources.v$PREV_VERSION\"" $BUNDLE_CSV
  yq e -i ".spec.install.spec.deployments.[0].spec.template.spec.containers[0].image=\"$OPERATOR_IMG\"" $BUNDLE_CSV
  yq e -i ".annotations.\"operators.operatorframework.io.bundle.package.v1\"=\"rhmi-cloud-resources\"" bundles/$VERSION/metadata/annotations.yaml
}

create_index() {
  yq e -i "select(.schema == \"olm.channel\").entries[0].name=\"cloud-resources.v$VERSION\"" index/base.yaml
  cp index/base.yaml index/index.yaml
}

start
