
# Validasi dependencies
DOCKER := $(shell command -v docker 2> /dev/null)
DOCKER_COMPOSE := $(shell command -v docker-compose 2> /dev/null)

.PHONY: help build up down logs clean restart

help: ## Show this help message
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the docker image
	docker-compose build

up: ## Start the application in detached mode
	docker-compose up -d

down: ## Stop the application
	docker-compose down

logs: ## Follow the logs of the application
	docker-compose logs -f

clean: ## Stop the application and remove volumes (WARNING: Deletes DB data)
	docker-compose down -v

restart: down up ## Restart the application

setup-env: ## Create .env from example if not exists
	cp -n .env.example .env || true
	@echo "Created .env file. Please edit it with your credentials."

test: ## Run Go tests
	go test ./...
