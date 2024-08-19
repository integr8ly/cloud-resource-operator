#!/usr/bin/env bash
CONTAINER_ENGINE ?= podman

if ${CONTAINER_ENGINE} pull ${IMAGE_TO_SCAN} > /dev/null; then
  echo "already exist"
else
  echo "image not present"
fi
