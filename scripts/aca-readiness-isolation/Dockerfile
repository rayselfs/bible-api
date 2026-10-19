FROM golang:1.24.6-alpine AS build
WORKDIR /src
COPY scripts/aca-readiness-isolation/fixture/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /fixture main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /fixture /fixture
USER nonroot:nonroot
ENTRYPOINT ["/fixture"]
