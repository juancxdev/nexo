FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/participation ./cmd/api
FROM alpine:3.22
COPY --from=build /out/participation /usr/local/bin/participation
EXPOSE 8084
ENTRYPOINT ["/usr/local/bin/participation"]
