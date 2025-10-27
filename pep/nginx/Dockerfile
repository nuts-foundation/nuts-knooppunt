# Knooppunt Policy Enforcement Point (PEP)
# NGINX with JavaScript module for authorization

FROM nginx:1.25-alpine

# Install envsubst for environment variable substitution
RUN apk add --no-cache gettext

# Copy NGINX configuration files
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/conf.d/ /etc/nginx/conf.d/
COPY nginx/js/ /etc/nginx/js/

# Create log directory
RUN mkdir -p /var/log/nginx && \
    chown -R nginx:nginx /var/log/nginx

# Set default environment variables
ENV FHIR_BACKEND_HOST=fhir \
    FHIR_BACKEND_PORT=8080 \
    KNOOPPUNT_PDP_HOST=knooppunt \
    KNOOPPUNT_PDP_PORT=8081

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

# Create entrypoint script to substitute environment variables
RUN echo '#!/bin/sh' > /docker-entrypoint.sh && \
    echo 'set -e' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Substitute environment variables in NGINX config' >> /docker-entrypoint.sh && \
    echo 'envsubst '\''$FHIR_BACKEND_HOST $FHIR_BACKEND_PORT $KNOOPPUNT_PDP_HOST $KNOOPPUNT_PDP_PORT'\'' \' >> /docker-entrypoint.sh && \
    echo '  < /etc/nginx/conf.d/knooppunt.conf.template \' >> /docker-entrypoint.sh && \
    echo '  > /etc/nginx/conf.d/knooppunt.conf' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Start NGINX' >> /docker-entrypoint.sh && \
    echo 'exec nginx -g "daemon off;"' >> /docker-entrypoint.sh && \
    chmod +x /docker-entrypoint.sh

# Rename config to template for envsubst
RUN mv /etc/nginx/conf.d/knooppunt.conf /etc/nginx/conf.d/knooppunt.conf.template

ENTRYPOINT ["/docker-entrypoint.sh"]
