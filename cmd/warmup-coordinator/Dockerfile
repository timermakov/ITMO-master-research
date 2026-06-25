FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY warmkit warmkit
COPY internal internal
COPY cmd/warmup-coordinator cmd/warmup-coordinator
WORKDIR /build/cmd/warmup-coordinator
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 go build -o /app/warmup-coordinator .

FROM alpine:3.20
COPY --from=builder /app/warmup-coordinator /app/warmup-coordinator
ENTRYPOINT ["/app/warmup-coordinator"]
