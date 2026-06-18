APP:=coldstart
PKG:=github.com/itmo-vkr/loadedservice

.PHONY: run build tidy fmt vet test

run:
	go run ./cmd/coldstart -addr=:8080 -file=data/big.bin -size=500 -samples=1000

build:
	go build -o bin/$(APP) ./cmd/coldstart

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...


