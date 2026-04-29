# --- Build stage: compile the React app ---
FROM node:18-alpine AS build
WORKDIR /app
ARG BASE_URL=
# CRA bakes PUBLIC_URL into asset paths and into process.env.PUBLIC_URL in the
# bundle, so it must be set at build time. Pass via `--build-arg BASE_URL=/ehr`.
ENV PUBLIC_URL=$BASE_URL
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# --- Runtime stage: minimal Node image serving the build + proxying FHIR ---
FROM node:18-alpine
WORKDIR /app
ENV NODE_ENV=production
ARG BASE_URL=
ENV BASE_URL=$BASE_URL
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY server.js proxy-allowlist.js ./
COPY --from=build /app/build ./build
EXPOSE 3000
CMD ["node", "server.js"]
