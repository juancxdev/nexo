FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /event-operations ./cmd/api
FROM alpine:3.22
RUN addgroup -S nexo && adduser -S -G nexo nexo
COPY --from=build /event-operations /event-operations
USER nexo
EXPOSE 8083
ENTRYPOINT ["/event-operations"]
