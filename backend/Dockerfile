FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/nexo-core-api ./cmd/api

FROM alpine:3.22
RUN adduser -D -H nexo
RUN mkdir -p /var/lib/nexo/keys && chown -R nexo:nexo /var/lib/nexo
USER nexo
COPY --from=build /out/nexo-core-api /usr/local/bin/nexo-core-api
VOLUME ["/var/lib/nexo/keys"]
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/nexo-core-api"]
