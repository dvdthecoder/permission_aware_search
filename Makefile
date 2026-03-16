SHELL := /bin/sh

.PHONY: help up up-build down logs ps test test-go test-semantic smoke clean-data

help:
	@echo "Targets:"
	@echo "  make up            - Start compose stack in detached mode"
	@echo "  make up-build      - Build and start compose stack in detached mode"
	@echo "  make down          - Stop compose stack"
	@echo "  make logs          - Tail compose logs"
	@echo "  make ps            - Show compose service status"
	@echo "  make test          - Run full Go test suite"
	@echo "  make test-semantic - Run semantic + API integration focused tests"
	@echo "  make smoke         - Smoke test /api/query/interpret"
	@echo "  make clean-data    - Remove local SQLite demo DB files"

up:
	docker compose up -d

up-build:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

ps:
	docker compose ps

test: test-go

test-go:
	go test ./...

test-semantic:
	go test ./cmd/api ./internal/semantic

smoke:
	curl -sS -X POST http://127.0.0.1:8080/api/query/interpret \
	  -H 'Content-Type: application/json' \
	  -H 'X-User-Id: alice' \
	  -H 'X-Tenant-Id: tenant-a' \
	  -d '{"message":"show open orders this week","provider":"slm-superlinked","contractVersion":"v2","debug":true}'

clean-data:
	rm -f data/*.db data/*.sqlite
