FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY warmkit warmkit
COPY internal internal
COPY LoadedService LoadedService
WORKDIR /build/LoadedService
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /app/server /app/server
ENTRYPOINT ["/app/server"]
