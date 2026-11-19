FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -o /hukid-simulator ./cmd/api
FROM alpine:3.22
COPY --from=build /hukid-simulator /hukid-simulator
EXPOSE 8090
ENTRYPOINT ["/hukid-simulator"]
