.PHONY: help run build test test-race coverage coverage-html vet lint tidy clean \
        docker-up docker-down schema-apply schema-init migration-apply migration-diff swagger \
        k8s-preview k8s-apply k8s-diff dev all

.DEFAULT_GOAL := help

-include .env
export

DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

## help: show all targets
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## run: start server locally
run:
	go run ./cmd/server

## build: compile binary
build:
	CGO_ENABLED=0 go build -o bin/ewallet-api ./cmd/server

## test: run all tests
test:
	go test $$(go list ./... | grep -v '/test$$') -count=1

## test-race: run all tests with race detector
test-race:
	go test $$(go list ./... | grep -v '/test$$') -race -count=1

## test-e2e: run E2E integration tests (requires running server)
test-e2e:
	go test ./test -v -count=1

## test-v: run tests verbose
test-v:
	go test $$(go list ./... | grep -v '/test$$') -v -count=1

## coverage: test coverage report (func)
coverage:
	go test $$(go list ./... | grep -v '/test$$') -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out

## coverage-html: open coverage report in browser
coverage-html:
	go test $$(go list ./... | grep -v '/test$$') -coverprofile=coverage.out -count=1
	go tool cover -html=coverage.out

## vet: run go vet
vet:
	go vet ./...

## lint: vet + check compilation
lint: vet
	go build ./...

## tidy: clean up go.mod
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf bin/ coverage.out

## swagger: regenerate swagger docs
swagger:
	swag init -g internal/handler/wallet.go -o docs --parseDependency

## docker-up: start postgres + otel-collector
docker-up:
	docker compose up -d

## docker-down: stop all services
docker-down:
	docker compose down

## docker-logs: tail all container logs
docker-logs:
	docker compose logs -f

## migration-apply: apply HCL schema via atlas (declarative)
migration-apply:
	atlas schema apply \
		--env local \
		--config file://migrations/atlas.hcl \
		--auto-approve

## migration-inspect: regenerate HCL from current DB state
migration-inspect:
	atlas schema inspect \
		--env local \
		--config file://migrations/atlas.hcl \
		--format '{{ hcl . }}' > migrations/schema.pg.hcl

## migration-diff: diff current DB vs HCL desired state
migration-diff:
	atlas schema diff \
		--env local \
		--config file://migrations/atlas.hcl

## schema-apply: apply raw schema.sql to running postgres
schema-apply:
	docker compose exec -T postgres psql -U postgres -d ewallet < migrations/schema.sql

## schema-init: fresh db — drop, create, apply schema
schema-init:
	docker compose exec -T postgres psql -U postgres -c "DROP DATABASE IF EXISTS ewallet"
	docker compose exec -T postgres psql -U postgres -c "CREATE DATABASE ewallet"
	$(MAKE) schema-apply

## k8s-preview: preview production kustomize output
k8s-preview:
	kubectl kustomize k8s/overlays/production

## k8s-apply: apply production overlay
k8s-apply:
	kubectl apply -k k8s/overlays/production

## k8s-diff: diff production against cluster
k8s-diff:
	kubectl diff -k k8s/overlays/production

## setup-all: docker-up + migration-apply (setup env, then make run)
setup-all: docker-up
	sleep 3
	$(MAKE) migration-apply

## dev: docker-up + migration-apply + run (full local dev)
dev: docker-up
	sleep 3
	$(MAKE) migration-apply
	$(MAKE) run

## all: lint + test + build
all: lint test build
