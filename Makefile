COMPOSE := docker compose
GO_IMAGE := golang:1.23-alpine
APP_DIR := /app
.DEFAULT_GOAL := help

.PHONY: help dev up build restart down reset logs ps test tidy fmt shell db redis db-ui redis-ui health lead leads score

help:
	@echo "lead-scoring shortcuts"
	@echo ""
	@echo "  make dev       Build and start the full local stack"
	@echo "  make up        Start existing containers without rebuilding"
	@echo "  make build     Rebuild the API image"
	@echo "  make restart   Restart API after code/config changes"
	@echo "  make down      Stop containers"
	@echo "  make reset     Stop containers and delete local DB volume"
	@echo "  make logs      Tail API logs"
	@echo "  make ps        Show containers"
	@echo "  make test      Run Go tests inside Docker"
	@echo "  make tidy      Run go mod tidy inside Docker"
	@echo "  make fmt       Run gofmt inside Docker"
	@echo "  make shell     Shell into API container"
	@echo "  make db        Open psql in Postgres container"
	@echo "  make redis     Open redis-cli in Redis container"
	@echo "  make health    Check API health"
	@echo "  make lead      Create a sample lead"
	@echo "  make leads     List cached leads"
	@echo "  make score     Score LEAD_ID=<id> with RAG"

dev:
	$(COMPOSE) up --build -d

up:
	$(COMPOSE) up -d

build:
	$(COMPOSE) build api

restart:
	$(COMPOSE) up --build --force-recreate -d api

down:
	$(COMPOSE) down

reset:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f api

ps:
	$(COMPOSE) ps

test:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) go test ./...

tidy:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) go mod tidy

fmt:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) gofmt -w cmd internal

shell:
	$(COMPOSE) exec api sh

db:
	$(COMPOSE) exec postgres psql -U root -d lead_scoring

redis:
	$(COMPOSE) exec redis redis-cli

db-ui:
	@echo "Postgres UI: http://localhost:8081"

redis-ui:
	@echo "Redis UI: http://localhost:8082"

health:
	$(COMPOSE) exec api wget -qO- http://localhost:8080/healthz

lead:
	$(COMPOSE) exec api wget -qO- --header 'Content-Type: application/json' --header 'Idempotency-Key: make-shortcut-lead' --post-data '{"company_name":"Shortcut Test Co","email":"buyer@shortcut.example","source":"make"}' http://localhost:8080/v1/create-leads

leads:
	$(COMPOSE) exec api wget -qO- 'http://localhost:8080/v1/get-leads?limit=10&offset=0'

score:
	@test -n "$(LEAD_ID)" || (echo "usage: make score LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- --post-data '' http://localhost:8080/v1/leads/$(LEAD_ID)/score
