.PHONY: test vet lint build tidy profile-cpu profile-flame profile-heap load compose-up compose-down bench-s0 bench-s-ref bench-s-dw bench-h4 bench-all figures

ifneq (,$(wildcard .env.local))
include .env.local
export
endif

GOLANGCI_LINT_CACHE ?= $(CURDIR)/.cache/golangci-lint

test:
	cd warmkit && go test ./...
	cd internal/envcfg && go test ./...
	cd internal/pprofserver && go test ./...
	cd LoadedService && go test ./...
	cd cmd/mirror-proxy && go test ./...
	cd cmd/warmup-coordinator && go test ./...
	cd benchmark-bench && go test ./...

vet:
	cd warmkit && go vet ./...
	cd internal/envcfg && go vet ./...
	cd internal/pprofserver && go vet ./...
	cd LoadedService && go vet ./...
	cd cmd/mirror-proxy && go vet ./...
	cd cmd/warmup-coordinator && go vet ./...
	cd benchmark-bench && go vet ./...

lint:
	cd warmkit && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd internal/envcfg && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd internal/pprofserver && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd LoadedService && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd cmd/mirror-proxy && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd cmd/warmup-coordinator && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...
	cd benchmark-bench && GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE)" golangci-lint run ./...

build:
	cd LoadedService && go build -o ../bin/loadedservice ./cmd/server
	cd cmd/mirror-proxy && go build -o ../../bin/mirror-proxy .
	cd cmd/warmup-coordinator && go build -o ../../bin/warmup-coordinator .
	cd benchmark-bench && go build -o ../bin/bench ./cmd/bench

tidy:
	go work sync
	cd warmkit && go mod tidy
	cd internal/envcfg && go mod tidy
	cd internal/pprofserver && go mod tidy
	cd LoadedService && go mod tidy
	cd cmd/mirror-proxy && go mod tidy
	cd cmd/warmup-coordinator && go mod tidy
	cd benchmark-bench && go mod tidy

profile-cpu:
	@test -n "$(DWSS_PPROF_HTTP_ADDR)" || (echo "DWSS_PPROF_HTTP_ADDR required in .env.local" && exit 1)
	@test -n "$(DWSS_PPROF_PROFILE_SECONDS)" || (echo "DWSS_PPROF_PROFILE_SECONDS required" && exit 1)
	mkdir -p results
	go tool pprof -raw -output=results/cpu.txt \
		"http://$(DWSS_PPROF_HTTP_ADDR)/debug/pprof/profile?seconds=$(DWSS_PPROF_PROFILE_SECONDS)"

profile-flame:
	@test -n "$(DWSS_PPROF_HTTP_ADDR)" || (echo "DWSS_PPROF_HTTP_ADDR required in .env.local" && exit 1)
	@test -n "$(DWSS_PPROF_PROFILE_SECONDS)" || (echo "DWSS_PPROF_PROFILE_SECONDS required" && exit 1)
	@test -n "$(DWSS_PPROF_UI_ADDR)" || (echo "DWSS_PPROF_UI_ADDR required" && exit 1)
	go tool pprof -http=$(DWSS_PPROF_UI_ADDR) \
		"http://$(DWSS_PPROF_HTTP_ADDR)/debug/pprof/profile?seconds=$(DWSS_PPROF_PROFILE_SECONDS)"

profile-heap:
	@test -n "$(DWSS_PPROF_HTTP_ADDR)" || (echo "DWSS_PPROF_HTTP_ADDR required in .env.local" && exit 1)
	mkdir -p results
	go tool pprof -raw -output=results/heap.txt \
		"http://$(DWSS_PPROF_HTTP_ADDR)/debug/pprof/heap"

compose-up:
	docker compose -f deploy/docker-compose.yml --env-file .env.local up --build

compose-down:
	docker compose -f deploy/docker-compose.yml --env-file .env.local down

bench-s0:
	cd benchmark-bench && go run ./cmd/bench run s0-control --out ../results/s0

bench-s-ref:
	cd benchmark-bench && go run ./cmd/bench run s-ref --out ../results/s-ref

bench-s-dw:
	cd benchmark-bench && go run ./cmd/bench run s-dw --out ../results/s-dw

bench-h4:
	cd benchmark-bench && go run ./cmd/bench run h4-overhead --out ../results/h4

bench-all:
	cd benchmark-bench && go run ./cmd/bench run all --out ../results/full

figures:
	python -m pip install -q -r scripts/benchmark-figures/requirements.txt
	python scripts/benchmark-figures/generate_all.py --out figures
