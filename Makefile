# IncidentOps Arena
#
# Everything here runs offline. No target reaches a network except `image`, which
# pulls its base images.

SHELL      := /bin/sh
VERSION    ?= 0.1.0-mvp
GO         ?= go
BIN        := bin
COMPOSE    := docker compose -f deploy/docker/docker-compose.yml
LDFLAGS    := -s -w -X main.Version=$(VERSION)

.DEFAULT_GOAL := help

## help: list the available targets
help:
	@echo "IncidentOps Arena $(VERSION)"
	@echo
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-18s %s\n", $$1, $$2}'

## build: compile every command into ./bin
build:
	@mkdir -p $(BIN)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN)/arena          ./cmd/arena
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN)/evalctl        ./cmd/evalctl
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN)/faultctl       ./cmd/faultctl
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN)/sddctl         ./cmd/sddctl
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BIN)/demo-order-api ./cmd/demo-order-api
	@echo "built $(BIN)/{arena,evalctl,faultctl,sddctl,demo-order-api}"

## test: run every test offline
test:
	$(GO) test ./...

## test-race: run every test under the race detector
test-race:
	$(GO) test -race ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## fmt: format every source file
fmt:
	$(GO) fmt ./...

## check: fmt, vet, tests and governance — the gate M1 must pass
check: vet test sdd-validate
	@echo
	@echo "M1 gate: build, vet, tests and governance are green"

## demo: run the reference scenario end to end and print its report
demo:
	$(GO) run ./cmd/arena demo --case C1

## demo-gate: run the reference scenario and stop at the approval gate
demo-gate:
	$(GO) run ./cmd/arena demo --case C1 --approve=false

## eval: compare the three flows over the whole fault case library
eval:
	$(GO) run ./cmd/evalctl run

## eval-report: write the comparison to docs/evaluation.md
eval-report:
	$(GO) run ./cmd/evalctl run --out docs/evaluation.md

## serve: run the API and console on :8080 with an in-memory store
serve:
	$(GO) run ./cmd/arena serve --addr :8080 --store memory

## sdd-validate: bilingual parity, traceability and drift
sdd-validate:
	$(GO) run ./cmd/sddctl validate

## sdd-gate: assert the delivery stage preconditions
sdd-gate:
	$(GO) run ./cmd/sddctl gate --stage deliver

## sdd-matrix: regenerate the traceability matrix
sdd-matrix:
	$(GO) run ./cmd/sddctl matrix --out docs/traceability-matrix.md

## sdd-seal: accept the current specification tree as the baseline
sdd-seal:
	$(GO) run ./cmd/sddctl seal

## sdd-impact: print the blast radius of changing an item, e.g. make sdd-impact ID=G-002
sdd-impact:
	@test -n "$(ID)" || (echo "usage: make sdd-impact ID=G-002" && exit 2)
	$(GO) run ./cmd/sddctl impact $(ID)

## image: build the container image
image:
	docker build -f deploy/docker/Dockerfile --build-arg VERSION=$(VERSION) \
		-t incidentops-arena:$(VERSION) .

## up: start the demo stack
up:
	$(COMPOSE) up --build -d
	@echo "console: http://localhost:8080   demo service: http://localhost:8081   prometheus: http://localhost:9090"

## down: stop the demo stack and remove its volumes
down:
	$(COMPOSE) down -v

## logs: follow the demo stack logs
logs:
	$(COMPOSE) logs -f

## inject: apply the C1 configuration fault to the demo service
inject:
	$(GO) run ./cmd/faultctl inject --case C1

## restore: put the C1 configuration back
restore:
	$(GO) run ./cmd/faultctl restore --case C1

## clean: remove build output
clean:
	rm -rf $(BIN) data

.PHONY: help build test test-race vet fmt check demo demo-gate eval eval-report \
        serve sdd-validate sdd-gate sdd-matrix sdd-seal sdd-impact image up down \
        logs inject restore clean
