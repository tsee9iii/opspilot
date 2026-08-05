.PHONY: all tidy fmt vet lint test build build-central build-agent \
        run-central run-agent \
        dev-up dev-down sqlc-generate clean help

SHELL := /bin/bash

BIN_DIR := bin
CENTRAL_BIN := $(BIN_DIR)/central
AGENT_BIN := $(BIN_DIR)/agent

all: fmt vet build

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

build: build-central build-agent

build-central:
	go build -o $(CENTRAL_BIN) ./cmd/central

build-agent:
	go build -o $(AGENT_BIN) ./cmd/agent

test:
	go test ./...

run-central:
	go run ./cmd/central

run-agent:
	go run ./cmd/agent

dev-up:
	docker compose -f deployments/docker-compose.yml up -d --build

dev-down:
	docker compose -f deployments/docker-compose.yml down

sqlc-generate:
	sqlc generate

clean:
	rm -rf $(BIN_DIR)

help:
	@grep -E '^[a-zA-Z_-]+:' Makefile | sed 's/://' | sort
