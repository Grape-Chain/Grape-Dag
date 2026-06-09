# SPDX-License-Identifier: Apache-2.0
#
# Multi-stage build for the Grape SMC VM. Produces an image with the
# server module's runnable jar plus its runtime dependencies.

FROM maven:3.9-eclipse-temurin-17 AS builder

WORKDIR /build

COPY pom.xml ./
COPY grape1utils/pom.xml      grape1utils/pom.xml
COPY grap3-crypto/pom.xml    grap3-crypto/pom.xml
COPY grap3-ether/pom.xml     grap3-ether/pom.xml
COPY commons/pom.xml         commons/pom.xml
COPY l1vm/pom.xml            l1vm/pom.xml
COPY vm/pom.xml              vm/pom.xml
COPY server/pom.xml          server/pom.xml
COPY grapech/pom.xml          grapech/pom.xml

RUN --mount=type=cache,target=/root/.m2 \
    mvn -B -ntp -T 1C dependency:go-offline -DskipTests || true

COPY . .

RUN --mount=type=cache,target=/root/.m2 \
    mvn -B -ntp -T 1C clean install -DskipTests

RUN cp server/target/server-*.jar /build/server.jar


FROM eclipse-temurin:17-jre-alpine AS runtime

RUN apk add --no-cache tini && \
    addgroup -g 1000 grape && \
    adduser -D -u 1000 -G grape grape

WORKDIR /app
COPY --from=builder /build/server/target/lib ./lib
COPY --from=builder /build/server.jar ./vm-engine-server.jar

USER grape

EXPOSE 29299 39399

ENTRYPOINT ["/sbin/tini", "--", "java", "-jar", "/app/vm-engine-server.jar"]
