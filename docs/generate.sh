#!/bin/bash
docker run --rm -v "${PWD}:/docs:ro" -v "${PWD}/diagrams:/diagrams" \
  structurizr/cli:latest \
  export \
  -workspace /docs/c4-diagram.dsl \
  -format plantuml/structurizr \
  -output /diagrams