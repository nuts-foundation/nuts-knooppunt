docker run --rm -v "${PWD}:/docs:ro" -v ./images:/images \
  extenda/structurizr-to-png \
  --path c4-diagram.dsl \
  --output /images