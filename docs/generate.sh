#!/bin/bash
set -euo pipefail

# if images_im exists, remove it
if [ -d "${PWD}/images_im" ]; then
  rm -rf "${PWD}/images_im"
fi

docker run --rm -v "${PWD}:/docs:ro" -v "${PWD}/images_im:/diagrams" \
  structurizr/cli:latest \
  export \
  -workspace /docs/c4-diagram.dsl \
  -format plantuml/c4plantuml \
  -output /diagrams \

# Post-processing: convert generated PlantUML files to SVG using PlantUML Docker image.
# This will look for files with .puml or .plantuml extensions under docs/diagrams
# and run PlantUML to produce .svg files alongside them.

docker run --rm -v "${PWD}/images_im:/diagrams:ro" -v "${PWD}/images:/images" plantuml/plantuml:latest \
 plantuml -verbose -tsvg  -o /images /diagrams
