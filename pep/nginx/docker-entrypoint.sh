#!/bin/sh
set -e

# Substitute environment variables in NGINX config
envsubst '$FHIR_BACKEND_HOST $FHIR_BACKEND_PORT $KNOOPPUNT_PDP_HOST $KNOOPPUNT_PDP_PORT' \
  < /etc/nginx/conf.d/knooppunt.conf.template \
  > /etc/nginx/conf.d/knooppunt.conf

# Start NGINX
exec nginx -g "daemon off;"
