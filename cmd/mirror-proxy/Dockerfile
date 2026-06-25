FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY warmkit warmkit
COPY internal internal
COPY cmd/mirror-proxy cmd/mirror-proxy
WORKDIR /build/cmd/mirror-proxy
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 go build -o /app/mirror-proxy .

FROM alpine:3.20
COPY --from=builder /app/mirror-proxy /app/mirror-proxy
ENTRYPOINT ["/app/mirror-proxy"]
