#!/usr/bin/env bash

if podman pull ${IMAGE_TO_SCAN} > /dev/null; then
  echo "already exist"
else
  echo "image not present"
fi