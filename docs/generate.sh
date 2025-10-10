#!/bin/bash
docker run --rm -v "${PWD}:/docs:ro" -v "${PWD}/images:/images" \
  extenda/structurizr-to-png \
  --path c4-diagram.dsl \
  --output /images