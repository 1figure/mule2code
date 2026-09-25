# mule/contacts-api on the community Mule Kernel (no MuleSoft account needed).
# Build context is the repository root: docker compose -f docker/docker-compose.yml --profile mule-ce build
#
# The batch app cannot be built this way: its Batch module is Enterprise Edition only.

ARG MULE_VERSION=4.12.0

# 1. Package the Mule application with the mule-maven-plugin (public MuleSoft repositories).
FROM maven:3.9-eclipse-temurin-17 AS app
WORKDIR /build
COPY mule/contacts-api/ .
RUN mvn -q -B -DskipTests package

# 2. Fetch the community runtime. org.mule.distributions:mule-standalone is the Kernel (CE).
FROM alpine:3 AS runtime
ARG MULE_VERSION
RUN wget -qO- "https://repository.mulesoft.org/nexus/content/repositories/releases/org/mule/distributions/mule-standalone/${MULE_VERSION}/mule-standalone-${MULE_VERSION}.tar.gz" \
      | tar xz -C /opt \
 && mv "/opt/mule-standalone-${MULE_VERSION}" /opt/mule

# 3. Runtime image: JRE 17 (mule-artifact.json: javaSpecificationVersions 17) + Kernel + the app.
FROM eclipse-temurin:17-jre
ENV MULE_HOME=/opt/mule
COPY --from=runtime /opt/mule /opt/mule
COPY --from=app /build/target/*-mule-application.jar /opt/mule/apps/contacts-api.jar
EXPOSE 8082
# Foreground; -M-D passes a system property to the runtime, selecting properties/mule-props-postgres.yaml.
CMD ["/opt/mule/bin/mule", "-M-Denv=postgres"]
