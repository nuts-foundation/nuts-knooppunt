# HAPI FHIR with the mock interceptors baked in (published as ghcr.io/nuts-foundation/fake-nvi):
# - nvi/hapi-pseudonimization-interceptor: the NVI, on partition NVI_TENANT;
# - lrza/hapi-lrza-interceptor: the LRZA's write rules, on partition LRZA_TENANT.
#
# Build from mock-components/:
#   docker build -f mock-components/hapi/Dockerfile mock-components

# The Maven builds run on the build host platform: the packaged jars are architecture-independent JVM
# bytecode, so there's no need to emulate them per target platform.
FROM --platform=$BUILDPLATFORM maven:3.8.3-openjdk-17 AS nvi
WORKDIR /app
COPY nvi/hapi-pseudonimization-interceptor/pom.xml /app/pom.xml
RUN mvn -B dependency:go-offline
COPY nvi/hapi-pseudonimization-interceptor/src /app/src
RUN mvn -B clean package -DskipTests

FROM --platform=$BUILDPLATFORM maven:3.8.3-openjdk-17 AS lrza
WORKDIR /app
COPY lrza/hapi-lrza-interceptor/pom.xml /app/pom.xml
RUN mvn -B dependency:go-offline
COPY lrza/hapi-lrza-interceptor/src /app/src
# Packages the jar, and in target/lib the dependencies the HAPI image lacks.
RUN mvn -B clean package -DskipTests

# The GF Care Services Directory IG and the packages it depends on, which the LRZA interceptor validates
# against (LRZA_PACKAGES_DIR). The dependencies are versioned releases; the IG is the build of its 1.0.0
# branch, as the Knooppunt targets (component/lrza).
FROM --platform=$BUILDPLATFORM curlimages/curl:8.10.1 AS packages
ARG CSD_PACKAGE_URL=https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/package.tgz
WORKDIR /tmp/packages
RUN curl -fsSL --retry 3 -o nl.generiekefuncties.csd-1.0.0.tgz "$CSD_PACKAGE_URL" \
 && for package in \
      hl7.terminology.r4/7.4.0 \
      hl7.fhir.uv.extensions.r4/5.3.0 \
      ihe.iti.mcsd/4.0.0 \
      hl7.fhir.eu.base/2.0.0 \
      hl7.fhir.eu.extensions.r4/1.3.0; do \
      curl -fsSL --retry 3 -o "$(echo "$package" | tr / -).tgz" "https://packages.fhir.org/$package" || exit 1; \
    done

FROM hapiproject/hapi:v8.2.0-2

COPY --from=nvi /app/target/*.jar /app/extra-classes/
COPY --from=lrza /app/target/*.jar /app/target/lib/*.jar /app/extra-classes/
COPY --from=packages /tmp/packages /app/lrza-packages
ENV LRZA_PACKAGES_DIR=/app/lrza-packages
ENV hapi.fhir.custom-bean-packages=nl.nuts.interceptor,nl.nuts.lrza
ENV hapi.fhir.custom-interceptor-classes=nl.nuts.interceptor.PseudonymInterceptor,nl.nuts.lrza.LrzaInterceptor
ENV hapi.fhir.allow_external_references=true
ENV hapi.fhir.partitioning.allow_references_across_partitions=false
