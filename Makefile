.PHONY: up down logs build build-web docker-build test test-backend test-parser test-integration lint lint-backend lint-parser venv-parser setup

PARSER_VENV := parser/.venv

up: ## Start the full stack locally
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f api worker parser

build:
	cd backend && go build ./...

build-web:
	cd web && npm ci && npm run build

docker-build:
	docker build --target api -t gok-api ./backend
	docker build --target worker -t gok-worker ./backend
	docker build -t gok-parser ./parser
	docker build -t gok-web ./web

# --- Tests ------------------------------------------------------------------

test: test-backend test-parser

test-backend:
	cd backend && go test -race ./...

test-parser: venv-parser
	cd parser && .venv/bin/python -m pytest tests/ -q

# Requires DATABASE_URL pointing at a disposable Postgres with pgvector,
# e.g. the docker-compose one:
#   DATABASE_URL=postgres://gok:gok_dev_password@localhost:5432/gok?sslmode=disable make test-integration
test-integration:
	cd backend && go test -race -tags integration ./...

# --- Lint -------------------------------------------------------------------

lint: lint-backend lint-parser

lint-backend:
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"

lint-parser: venv-parser
	cd parser && .venv/bin/ruff check app tests

# --- Python virtualenv ------------------------------------------------------
# The venv is created once and rebuilt only when requirements.txt changes
# (the .stamp file carries the make dependency). Requires the python3-venv
# system package — run `make setup` once on Debian/Ubuntu if it is missing.

venv-parser: $(PARSER_VENV)/.stamp

$(PARSER_VENV)/.stamp: parser/requirements.txt
	python3 -m venv $(PARSER_VENV)
	$(PARSER_VENV)/bin/python -m pip install --quiet --upgrade pip
	$(PARSER_VENV)/bin/python -m pip install --quiet -r parser/requirements.txt pytest ruff
	touch $@

# One-time host setup (Debian/Ubuntu). Kept separate from venv-parser so no
# regular target ever needs sudo — CI images and most dev machines already
# have python3-venv.
setup:
	sudo apt install -y python3-venv
