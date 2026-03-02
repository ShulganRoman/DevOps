FROM maven:3.9.9-eclipse-temurin-11 AS builder

WORKDIR /build

COPY spring-backend/pom.xml ./pom.xml

RUN mvn -q -DskipTests dependency:go-offline

COPY spring-backend/src ./src

RUN mvn -q -DskipTests clean package


FROM azul/zulu-openjdk-alpine:11.0.30-jre

WORKDIR /app

RUN addgroup -S nonroot && adduser -S nonroot -G nonroot

COPY --from=builder /build/target/*.jar /app/app.jar

USER nonroot

EXPOSE 8080

CMD ["java", "-jar", "/app/app.jar"]
