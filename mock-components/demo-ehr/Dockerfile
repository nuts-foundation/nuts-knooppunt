# --- Build stage: compile the React app ---
FROM node:18-alpine AS build
WORKDIR /app
# PUBLIC_URL=. makes CRA emit relative asset paths (./static/...). server.js
# then injects a <base href> at runtime so those paths resolve correctly under
# whatever sub-path BASE_URL points at — no rebuild per deployment.
ENV PUBLIC_URL=.
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# --- Runtime stage: minimal Node image serving the build + proxying FHIR ---
FROM node:18-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY server.js proxy-allowlist.js ./
COPY --from=build /app/build ./build
EXPOSE 3000
CMD ["node", "server.js"]
