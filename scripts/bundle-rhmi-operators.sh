#!/usr/bin/env bash

LATEST_VERSION=$(grep cloud-resource-operator packagemanifests/cloud-resource-operator.package.yaml | awk -F v '{print $2}')

CHANNEL="${CHANNEL:-alpha}"
ORG="${IMAGE_ORG:-integreatly}"
REG="${IMAGE_REG:-quay.io}"
BUILD_TOOL="${BUILD_TOOL:-podman}"
UPGRADE_CRO="${UPGRADE:-true}"
VERSIONS="${BUNDLE_VERSIONS:-$LATEST_VERSION}"
ROOT=$(pwd)
INDEX_IMAGE=""
PREVIOUS_OPERATOR_VERSIONS="${PREVIOUS_OPERATOR_VERSIONS}"


start() {
  clean_up
  create_work_area
  copy_bundles
  check_upgrade_install
  generate_bundles
  push_index
  clean_up
}

create_work_area() {
  printf "Creating Work Area \n"

  cd ./packagemanifests/
  mkdir temp && cd temp
}

copy_bundles() {
  for i in $(echo $VERSIONS | sed "s/,/ /g")
  do
      printf 'Copying bundle version: \n'$i
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

  file=`ls './'$OLDEST_VERSION | grep .clusterserviceversion.yaml`

  sed '/replaces/d' './'$OLDEST_VERSION'/'$file > newfile ; mv newfile './'$OLDEST_VERSION'/'$file
}

# Generates a bundle for each of the version specified or, the latest version if no BUNDLE_VERSIONS  specified
generate_bundles() {
  printf "Generating Bundle \n"

  cd ./$LATEST_VERSION
  opm alpha bundle generate -d . --channels $CHANNEL \
      --package rhmi-cloud-resources --output-dir bundle \
      --default $CHANNEL

  ${BUILD_TOOL} build -f bundle.Dockerfile -t $REG/$ORG/cloud-resource-operator:bundle-v$LATEST_VERSION .
  ${BUILD_TOOL} push $REG/$ORG/cloud-resource-operator:bundle-v$LATEST_VERSION
  operator-sdk bundle validate $REG/$ORG/cloud-resource-operator:bundle-v$LATEST_VERSION
  cd ..
}

# builds and pushes the index for each version included
push_index() {
  bundles=""
  bundles=$bundles"$REG/$ORG/cloud-resource-operator:bundle-v$LATEST_VERSION,"
  for VERSION in $(echo $PREVIOUS_OPERATOR_VERSIONS | sed "s/,/ /g")
  do
      bundles=$bundles"$REG/$ORG/cloud-resource-operator:bundle-v$VERSION,"
  done
  bundles=${bundles%?}

  opm index add \
      --bundles $bundles \
      --build-tool ${BUILD_TOOL} \
      --tag $REG/$ORG/cloud-resource-operator:index-v$LATEST_VERSION

  INDEX_IMAGE=$REG/$ORG/cloud-resource-operator:index-v$LATEST_VERSION

  printf 'Pushing index image:'$INDEX_IMAGE'\n'

  ${BUILD_TOOL} push $INDEX_IMAGE
}

# cleans up the working space
clean_up() {
  printf 'Cleaning up work area \n'
  rm -rf $ROOT/packagemanifests/temp
}

start