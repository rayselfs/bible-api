FROM golang:1.24.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6 \
 && swag init -g cmd/main.go \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bible-api ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bible-api /bible-api
USER nonroot:nonroot
ENTRYPOINT ["/bible-api"]
