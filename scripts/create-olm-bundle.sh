#!/usr/bin/env bash

LATEST_VERSION=$(grep cloud-resource-operator bundles/cloud-resource-operator.package.yaml | awk -F v '{print $2}')

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
    operator-sdk bundle validate $image
  done
  cd $ROOT
}

# builds and pushes the index for each version included
generate_index() {

  if [ "$UPGRADE_CRO" = true ] ; then
    # Once the first file-based index has been built, we can render the PREV_VERSION to generate the new
    # version instead of iterating over all of the bundles (https://issues.redhat.com/browse/MGDAPI-3780)
    # opm render $REG/$ORG/cloud-resource-operator:index-v$PREV_VERSION -o yaml > index/index.yaml
    for VERSION in $(echo $PREVIOUS_OPERATOR_VERSIONS | sed "s/,/ /g")
    do
      render_bundle $VERSION
    done
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
  yq e -e "select(.schema == \"olm.bundle\") | select(.name == \"cloud-resources.v$1\").image" \
    $INDEX_FILE > /dev/null 2>&1

  # If the bundle doesn't exist in the index add it
  if [ $? -eq 1 ]; then
    printf "Rendering bundle for v$1\n"
    file=`ls bundles/$1/manifests | grep .clusterserviceversion.yaml`
    BUNDLE_CSV="bundles/$1/manifests/$file"

    # Update channel entries
    # check if this channel entry exists
    yq e -e "select(.schema == \"olm.channel\").entries[] | select(.name == \"cloud-resources.v$1\")" \
      $INDEX_FILE > /dev/null 2>&1
    if [ $? -eq 1 ]; then
      # channel entry for this version doesn't exist, create it
      yq e -i "select(.schema == \"olm.channel\").entries \
        |= . + {\"name\":\"cloud-resources.v$1\"}" \
        $INDEX_FILE
    fi

    # check if bundle has replaces
    yq e -e '.spec.replaces' $BUNDLE_CSV > /dev/null 2>&1
    if [ $? -eq 0 ]; then
      # set channel replaces from bundle
      yq ea -i "(select(fi==1) | .spec.replaces) as \$replaces \
        | select(fi==0) | select(.schema == \"olm.channel\").entries[] \
        |= select(.name == \"cloud-resources.v$1\").replaces=\$replaces" \
        $INDEX_FILE $BUNDLE_CSV
    fi

    # check if bundle has skips
    yq e -e '.spec.skips' $BUNDLE_CSV > /dev/null 2>&1
    if [ $? -eq 0 ]; then
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