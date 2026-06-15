.PHONY: help run start stop restart logs build test fmt service agent generator webview examples-test example-service example-agent example-webview example-webview-down compose-up compose-down terraform-init terraform-plan

COMPOSE ?= docker compose
SERVICE_ADDR ?= :8080
AGENT_UDP_ADDR ?= :8125
SERVICE_INGEST_URL ?= http://localhost:8080/v1/metrics
GENERATOR_RATE_PER_SECOND ?= 8
GOENV := GOWORK=off GOCACHE=$(CURDIR)/.gocache

help:
	@printf "custom-business-metrics MVP\n\n"
	@printf "Targets:\n"
	@printf "  make run             Start service, agent, generator and webview with Docker\n"
	@printf "  make stop            Stop local Docker stack\n"
	@printf "  make logs            Follow local stack logs\n"
	@printf "  make test            Run Go tests for service, agent and testapp\n"
	@printf "  make fmt             Format Go code\n"
	@printf "  make service         Run service locally\n"
	@printf "  make agent           Run agent locally\n"
	@printf "  make generator       Run synthetic metric generator locally\n"
	@printf "  make examples-test   Validate the importable examples\n"
	@printf "  make example-service Run the importable service example\n"
	@printf "  make example-agent   Send metrics with the importable agent example\n"
	@printf "  make example-webview Start the complete examples/webview stack\n"
	@printf "  make terraform-plan  Plan AWS infrastructure using infra/config/dev.tfvars\n"

run: compose-up

start: compose-up

compose-up:
	$(COMPOSE) up --build

stop: compose-down

compose-down:
	$(COMPOSE) down

restart:
	$(COMPOSE) down
	$(COMPOSE) up --build

logs:
	$(COMPOSE) logs -f

build:
	$(GOENV) go -C service build ./cmd/service
	$(GOENV) go -C agent build ./cmd/agent
	$(GOENV) go -C testapp build ./cmd/generator

test:
	$(GOENV) go -C service test ./...
	$(GOENV) go -C agent test ./...
	$(GOENV) go -C testapp test ./...

fmt:
	$(GOENV) go -C service fmt ./...
	$(GOENV) go -C agent fmt ./...
	$(GOENV) go -C testapp fmt ./...

service:
	SERVICE_ADDR=$(SERVICE_ADDR) $(GOENV) go -C service run ./cmd/service

agent:
	AGENT_UDP_ADDR=$(AGENT_UDP_ADDR) SERVICE_INGEST_URL=$(SERVICE_INGEST_URL) $(GOENV) go -C agent run ./cmd/agent

generator:
	AGENT_UDP_ADDR=localhost:8125 GENERATOR_RATE_PER_SECOND=$(GENERATOR_RATE_PER_SECOND) $(GOENV) go -C testapp run ./cmd/generator

webview:
	python3 -m http.server 5173 --directory webview

examples-test:
	$(GOENV) go -C examples/importable-agent test ./...
	$(GOENV) go -C examples/importable-service test ./...

example-service:
	$(GOENV) go -C examples/importable-service run .

example-agent:
	$(GOENV) go -C examples/importable-agent run .

example-webview:
	$(COMPOSE) -f examples/webview/docker-compose.yml up --build

example-webview-down:
	$(COMPOSE) -f examples/webview/docker-compose.yml down

terraform-init:
	terraform -chdir=infra init

terraform-plan:
	terraform -chdir=infra plan -var-file=config/dev.tfvars
