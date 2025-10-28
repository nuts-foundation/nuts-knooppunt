# Knooppunt Policy Enforcement Point (PEP)
# NGINX with JavaScript module for authorization

FROM nginx:1.29.2-alpine

# Copy NGINX configuration files
COPY nginx.conf /etc/nginx/nginx.conf
COPY js/authorize.js /etc/nginx/js/authorize.js

# Copy config template - official nginx image will process this automatically
# via /docker-entrypoint.d/20-envsubst-on-templates.sh
COPY conf.d/knooppunt.conf /etc/nginx/templates/knooppunt.conf.template

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
