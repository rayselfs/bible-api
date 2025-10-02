FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS base

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download Go module dependencies
RUN go mod download

# Build stage
FROM base AS build

# Swag setting
RUN apk add --no-cache git
RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN export PATH=$(go env GOPATH)/bin:$PATH

# Copy the source code to the working directory
COPY . .

RUN swag init -g cmd/main.go

# Build the Go application
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /app/main cmd/main.go

# STAGE 2: build the container to run
FROM gcr.io/distroless/static-debian12:latest AS final

# Set the working directory
WORKDIR /app

# Copy the binary from the build stage
COPY --chown=nonroot:nonroot --from=build /app/main /app/main

USER nonroot:nonroot

# Set the entrypoint command for the container
ENTRYPOINT ["/app/main"]
