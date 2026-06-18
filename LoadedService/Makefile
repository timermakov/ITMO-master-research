APP:=loadedservice
PKG:=github.com/itmo-vkr/dwss/LoadedService

ifneq (,$(wildcard ../.env.local))
include ../.env.local
export
endif

.PHONY: run build tidy fmt vet test

run:
	go run ./cmd/server

build:
	go build -o bin/$(APP) ./cmd/server

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...
