FROM maven:3.8.3-openjdk-17 AS builder

WORKDIR /app
COPY ./pom.xml /app/pom.xml
RUN mvn dependency:go-offline

COPY ./src /app/src
RUN mvn clean package -DskipTests

FROM hapiproject/hapi:v8.2.0-2

COPY --from=builder /app/target/*.jar /app/extra-classes/
ENV hapi.fhir.custom-bean-packages=nl.nuts.interceptor
ENV hapi.fhir.custom-interceptor-classes=nl.nuts.interceptor.PseudonymInterceptor
ENV hapi.fhir.allow_external_references=true
ENV hapi.fhir.partitioning.allow_references_across_partitions=false