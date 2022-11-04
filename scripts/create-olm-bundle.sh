#!/usr/bin/env bash

set -e
set -o pipefail

LATEST_VERSION=$(grep cloud-resource-operator bundles/cloud-resource-operator.package.yaml | awk -F v '{print $2}')

ORG="${IMAGE_ORG:-integreatly}"
REG="${IMAGE_REG:-quay.io}"
BUILD_TOOL="${BUILD_TOOL:-docker}"
UPGRADE_CRO="${UPGRADE:-true}"
VERSIONS="${BUNDLE_VERSIONS:-$LATEST_VERSION}"
ROOT=$(pwd)
INDEX_IMAGE=""

start() {
  clean_up
  create_work_area
  copy_bundles
  check_upgrade_install
  generate_bundles
  generate_index
  clean_up
}

create_work_area() {
  printf "Creating Work Area\n"

  cd ./bundles/
  mkdir temp && cd temp
}

copy_bundles() {
  for i in $(echo $VERSIONS | sed "s/,/ /g")
  do
      printf "Copying bundle version: $i\n"
      cp -R ../$i ./
  done
}

# Remove the replaces field in the csv to allow for a single bundle install or an upgrade install. i.e.
# The install will not require a previous version to replace.
check_upgrade_install() {
  if [ "$UPGRADE_CRO" = true ] ; then
    # We can return as the csv will have the replaces field by default
    echo 'Not removing replaces field in CSV'
    return
  fi
  # Get the oldest version, example: VERSIONS="2.5,2.4,2.3" oldest="2.3"
  OLDEST_VERSION=${VERSIONS##*,}

  file=`ls './'$OLDEST_VERSION/manifests | grep .clusterserviceversion.yaml`

  sed '/replaces/d' './'$OLDEST_VERSION'/manifests/'$file > newfile ; mv newfile './'$OLDEST_VERSION'/manifests/'$file
}

# Generates a bundle for each of the version specified or, the latest version if no BUNDLE_VERSIONS  specified
generate_bundles() {
  printf "Generating Bundle\n"

  cd $ROOT
  for VERSION in $(echo $VERSIONS | sed "s/,/ /g")
  do
    image="$REG/$ORG/cloud-resource-operator:bundle-v$VERSION"
    $BUILD_TOOL build -f ./bundles/bundle.Dockerfile -t $image \
      --build-arg manifest_path=./bundles/temp/$VERSION/manifests \
      --build-arg metadata_path=./bundles/temp/$VERSION/metadata \
      --build-arg version=$VERSION .
    $BUILD_TOOL push $image
    operator-sdk bundle validate \
      --image-builder $BUILD_TOOL \
      $image
  done
  cd $ROOT
}

# builds and pushes the index for each version included
generate_index() {

  if [ "$UPGRADE_CRO" = true ] ; then
    # render from existing index
    opm render $REG/$ORG/cloud-resource-operator:index-v$PREV_VERSION -o yaml > index/index.yaml
  fi
  render_bundle $LATEST_VERSION

  opm validate index

  INDEX_IMAGE=$REG/$ORG/cloud-resource-operator:index-v$LATEST_VERSION

  printf "Building index image:$INDEX_IMAGE\n"
  ${BUILD_TOOL} build . -f index.Dockerfile -t $INDEX_IMAGE

  printf "Pushing index image:'$INDEX_IMAGE\n"
  ${BUILD_TOOL} push $INDEX_IMAGE
}

render_bundle() {
  INDEX_FILE=index/index.yaml

  # Check whether there is a bundle in the index for this version
  bundle_entry=$(yq e "select(.schema == \"olm.bundle\") | select(.name == \"cloud-resources.v$1\").image" \
    $INDEX_FILE)

  # If the bundle doesn't exist in the index add it
  if [ -z "$bundle_entry" ]; then
    printf "Rendering bundle for v$1\n"
    file=`ls bundles/$1/manifests | grep .clusterserviceversion.yaml`
    BUNDLE_CSV="bundles/$1/manifests/$file"

    # Update channel entries
    # check if this channel entry exists
    channel_entry=$(yq e "select(.schema == \"olm.channel\").entries[] | select(.name == \"cloud-resources.v$1\")" \
      $INDEX_FILE)
    if [ -z "$channel_entry" ]; then
      # channel entry for this version doesn't exist, create it
      yq e -i "select(.schema == \"olm.channel\").entries \
        |= . + {\"name\":\"cloud-resources.v$1\"}" \
        $INDEX_FILE
    fi

    # check if bundle has replaces
    replaces=$(yq e '.spec.replaces' $BUNDLE_CSV)
    if [ "$replaces" != "null" ]; then
      # set channel replaces from bundle
      yq ea -i "(select(fi==1) | .spec.replaces) as \$replaces \
        | select(fi==0) | select(.schema == \"olm.channel\").entries[] \
        |= select(.name == \"cloud-resources.v$1\").replaces=\$replaces" \
        $INDEX_FILE $BUNDLE_CSV
    fi

    # check if bundle has skips
    skips=$(yq e '.spec.skips' $BUNDLE_CSV)
    if [ "$skips" != "null" ]; then
      # set channel skips from bundle
      yq ea -i "(select(fi==1) | .spec.skips) as \$skips \
        | select(fi==0) | select(.schema == \"olm.channel\").entries[] \
        |= select(.name == \"cloud-resources.v$1\").skips=\$skips" \
        $INDEX_FILE $BUNDLE_CSV
    fi

    # Render olm.bundle entry
    opm render "$REG/$ORG/cloud-resource-operator:bundle-v$1" \
      --output=yaml \
      >> $INDEX_FILE
  fi
}


# cleans up the working space
clean_up() {
  printf 'Cleaning up work area \n'
  rm -rf $ROOT/bundles/temp
}


start