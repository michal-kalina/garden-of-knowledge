.PHONY: up down build test lint logs upgrade mod

up: ## Run the application in the background
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

upgrade:
	cd backend && go get -u ./...

mod:
	cd backend && go mod tidy
