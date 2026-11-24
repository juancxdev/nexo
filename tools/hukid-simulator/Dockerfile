FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -o /hukid-simulator ./cmd/api
FROM alpine:3.22
RUN addgroup -S nexo && adduser -S -G nexo nexo
COPY --from=build /hukid-simulator /hukid-simulator
USER nexo
EXPOSE 8090
ENTRYPOINT ["/hukid-simulator"]
