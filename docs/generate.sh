#!/bin/bash
set -euo pipefail

# if images_im exists, remove it
if [ -d "${PWD}/images_im" ]; then
  rm -rf "${PWD}/images_im"
fi

# Ensure required directories exist before running Docker to avoid root-owned auto-creation
mkdir -p "${PWD}/images_im" "${PWD}/images"
docker run --rm -v "${PWD}:/docs:ro" -v "${PWD}/images_im:/diagrams" \
  structurizr/cli:2025.05.28 \
  export \
  -workspace /docs/c4-diagram.dsl \
  -format plantuml/c4plantuml \
  -output /diagrams \

# Post-processing: convert generated PlantUML files to SVG using PlantUML Docker image.
# This will look for files with .puml or .plantuml extensions under docs/diagrams
# and run PlantUML to produce .svg files alongside them.
cp "${PWD}/"*.puml "${PWD}/images_im/"
docker run --rm -v "${PWD}/images_im:/diagrams:ro" -v "${PWD}/images:/images" plantuml/plantuml:sha-162ede3 \
 plantuml -verbose -tsvg  -o /images /diagrams
