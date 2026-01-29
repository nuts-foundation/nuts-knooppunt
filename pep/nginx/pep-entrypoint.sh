#!/bin/sh
set -e

# Start dnsmasq in the background
# It reads /etc/hosts and serves those entries via DNS on 127.0.0.1:53
# This allows NGINX to resolve host.docker.internal (added via --add-host)
dnsmasq --no-daemon --log-queries --log-facility=/dev/stdout &

# Run the original nginx entrypoint
exec /docker-entrypoint.sh "$@"
