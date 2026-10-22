# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tensora ./cmd/tensora

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tensora /usr/local/bin/tensora
ENV TENSORA_HTTP_ADDR=:8080 \
    TENSORA_CATALOG_PATH=/etc/tensora/catalog.yaml
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tensora"]
CMD ["api"]
