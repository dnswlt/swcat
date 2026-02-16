# --- Build Stage ---
FROM maven:3.9.9-eclipse-temurin-21 AS builder

# The protobuf plugin needs access to the proto files in the root
WORKDIR /app
COPY proto ./proto
COPY extensions/grpc ./extensions/grpc

WORKDIR /app/extensions/grpc

# Build the fat JAR. -DskipTests is used to speed up the build.
RUN mvn clean package -DskipTests

# --- Runtime Stage ---
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

# Copy the shaded JAR from the builder stage
COPY --from=builder /app/extensions/grpc/target/grpc-plugin-server-1.0-SNAPSHOT.jar /app/grpc-plugin-server.jar

# Default gRPC port
EXPOSE 50051

# Start the server without arguments as requested
ENTRYPOINT ["java", "-jar", "/app/grpc-plugin-server.jar"]
