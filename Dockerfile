FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS base

RUN apk add --no-cache git

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download Go module dependencies
RUN go mod download

# Build stage
FROM base AS build

# Copy the source code to the working directory
COPY . .

# Build the Go application
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /app/main cmd/main.go

# STAGE 2: build the container to run
FROM gcr.io/distroless/static-debian12:latest AS final

# Set the working directory
WORKDIR /app

# Copy the binary from the build stage
COPY --chown=nonroot:nonroot --from=build /app/main /app/main

# Expose the port your application listens on
EXPOSE 9999

# Set the entrypoint command for the container
ENTRYPOINT ["/app/main"]
