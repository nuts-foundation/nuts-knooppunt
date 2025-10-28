# Knooppunt Policy Enforcement Point (PEP)
# NGINX with JavaScript module for authorization

FROM nginx:1.29.2-alpine

# Install envsubst for environment variable substitution
RUN apk add --no-cache gettext

# Copy NGINX configuration files
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/conf.d/ /etc/nginx/conf.d/
COPY nginx/js/ /etc/nginx/js/

# Copy entrypoint script
COPY nginx/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

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

# Rename config to template for envsubst
RUN mv /etc/nginx/conf.d/knooppunt.conf /etc/nginx/conf.d/knooppunt.conf.template

ENTRYPOINT ["/docker-entrypoint.sh"]
