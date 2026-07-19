COMPOSE := docker compose
GO_IMAGE := golang:1.23-alpine
APP_DIR := /app
API_KEY ?= dev-lead-scoring-key
AUTH := --header 'Authorization: Bearer $(API_KEY)'
.DEFAULT_GOAL := help

.PHONY: help dev up build restart down reset logs ps test tidy fmt migrate shell db redis db-ui redis-ui health lead leads embedding similar score latest-score scores status job

help:
	@echo "lead-scoring shortcuts"
	@echo ""
	@echo "  make dev       Build and start the full local stack"
	@echo "  make up        Start existing containers without rebuilding"
	@echo "  make build     Rebuild API/worker images"
	@echo "  make restart   Restart API and worker after code/config changes"
	@echo "  make down      Stop containers"
	@echo "  make reset     Stop containers and delete local DB volume"
	@echo "  make logs      Tail API logs"
	@echo "  make ps        Show containers"
	@echo "  make test      Run Go tests inside Docker"
	@echo "  make tidy      Run go mod tidy inside Docker"
	@echo "  make fmt       Run gofmt inside Docker"
	@echo "  make migrate   Apply SQL migrations to the running database"
	@echo "  make shell     Shell into API container"
	@echo "  make db        Open psql in Postgres container"
	@echo "  make redis     Open redis-cli in Redis container"
	@echo "  make health    Check API health"
	@echo "  make lead      Create a sample lead"
	@echo "  make leads     List cached leads"
	@echo "  make embedding LEAD_ID=<id>  Refresh lead embedding"
	@echo "  make similar   Find similar leads for LEAD_ID=<id>"
	@echo "  make score     Enqueue score job for LEAD_ID=<id>"
	@echo "  make job       JOB_ID=<id> Get async job status"
	@echo "  make latest-score LEAD_ID=<id>  Get latest score"
	@echo "  make scores    List score history for LEAD_ID=<id>"
	@echo "  make status    LEAD_ID=<id> STATUS=won Update lead status"

dev:
	$(COMPOSE) up --build -d

up:
	$(COMPOSE) up -d

build:
	$(COMPOSE) build api worker

restart:
	$(COMPOSE) up --build --force-recreate -d api worker

down:
	$(COMPOSE) down

reset:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f api worker

ps:
	$(COMPOSE) ps

test:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) go test ./...

tidy:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) go mod tidy

fmt:
	docker run --rm -v "$(CURDIR):$(APP_DIR)" -w "$(APP_DIR)" $(GO_IMAGE) gofmt -w cmd internal

migrate:
	@set -e; for file in migrations/*.sql; do \
		echo "Applying $$file"; \
		$(COMPOSE) exec -T postgres psql -U root -d lead_scoring < "$$file"; \
	done

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
	$(COMPOSE) exec api wget -qO- $(AUTH) --header 'Content-Type: application/json' --header 'Idempotency-Key: make-shortcut-lead' --post-data '{"company_name":"Shortcut Test Co","email":"buyer@shortcut.example","source":"make"}' http://localhost:8080/v1/create-leads

leads:
	$(COMPOSE) exec api wget -qO- $(AUTH) 'http://localhost:8080/v1/get-leads?limit=10&offset=0'

embedding:
	@test -n "$(LEAD_ID)" || (echo "usage: make embedding LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) --post-data '' http://localhost:8080/v1/leads/$(LEAD_ID)/embeddings

similar:
	@test -n "$(LEAD_ID)" || (echo "usage: make similar LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) 'http://localhost:8080/v1/leads/$(LEAD_ID)/similar?limit=5'

score:
	@test -n "$(LEAD_ID)" || (echo "usage: make score LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) --post-data '' http://localhost:8080/v1/leads/$(LEAD_ID)/score

job:
	@test -n "$(JOB_ID)" || (echo "usage: make job JOB_ID=<job-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) http://localhost:8080/v1/jobs/$(JOB_ID)

latest-score:
	@test -n "$(LEAD_ID)" || (echo "usage: make latest-score LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) http://localhost:8080/v1/leads/$(LEAD_ID)/score

scores:
	@test -n "$(LEAD_ID)" || (echo "usage: make scores LEAD_ID=<lead-id>" && exit 1)
	$(COMPOSE) exec api wget -qO- $(AUTH) 'http://localhost:8080/v1/leads/$(LEAD_ID)/scores?limit=20'

status:
	@test -n "$(LEAD_ID)" || (echo "usage: make status LEAD_ID=<lead-id> STATUS=won" && exit 1)
	@test -n "$(STATUS)" || (echo "usage: make status LEAD_ID=<lead-id> STATUS=won" && exit 1)
	$(COMPOSE) exec api curl -sS -X PATCH $(AUTH) -H 'Content-Type: application/json' -d '{"status":"$(STATUS)"}' http://localhost:8080/v1/leads/$(LEAD_ID)
