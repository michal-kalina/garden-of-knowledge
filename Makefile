.PHONY: up down build test lint logs

up: ## Uruchom cały stack lokalnie
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f api worker parser

build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

lint:
	cd backend && go vet ./...
