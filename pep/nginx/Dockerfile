# Knooppunt Policy Enforcement Point (PEP)
# NGINX with JavaScript module for authorization

FROM nginx:1.29.2-alpine

# Install dnsmasq - serves /etc/hosts entries via DNS so NGINX can resolve them
RUN apk add --no-cache dnsmasq

# Copy NGINX configuration files
COPY nginx.conf /etc/nginx/nginx.conf
COPY js/authorize.js /etc/nginx/js/authorize.js

# Remove default nginx config
RUN rm -f /etc/nginx/conf.d/default.conf

# Copy config template - official nginx image will process this automatically
# via /docker-entrypoint.d/20-envsubst-on-templates.sh
COPY conf.d/knooppunt.conf /etc/nginx/templates/knooppunt.conf.template

# Copy entrypoint script that starts dnsmasq before nginx
COPY pep-entrypoint.sh /pep-entrypoint.sh
RUN chmod +x /pep-entrypoint.sh

# Create log directory
RUN mkdir -p /var/log/nginx && \
    chown -R nginx:nginx /var/log/nginx

# Set default environment variables
# DNS_RESOLVER uses 127.0.0.1 (dnsmasq) which reads /etc/hosts
ENV NGINX_PORT=8080 \
    FHIR_BACKEND_HOST=fhir \
    FHIR_BACKEND_PORT=7050 \
    FHIR_BASE_PATH=/fhir \
    KNOOPPUNT_PDP_HOST=knooppunt \
    KNOOPPUNT_PDP_PORT=8081 \
    NUTS_NODE_HOST=knooppunt \
    NUTS_NODE_INTERNAL_PORT=8081 \
    DATA_HOLDER_ORGANIZATION_URA=00000001 \
    DATA_HOLDER_FACILITY_TYPE=Z3 \
    DNS_RESOLVER="127.0.0.1"

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

# Use custom entrypoint that starts dnsmasq before nginx
ENTRYPOINT ["/pep-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
