.PHONY: generate openapi ts proto tidy docker-build

ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

generate: openapi ts proto spec-copy proto-prune tidy

openapi:
	cd $(ROOT)services/backend && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 -config oapi-codegen.yaml ../../openapi/openapi.yaml

ts:
	npx --yes openapi-typescript@$(OT_VERSION) $(ROOT)openapi/openapi.yaml -o $(ROOT)packages/ts-client-generated/schema.d.ts

OT_VERSION ?= 7.4.4

proto:
	cd $(ROOT)proto && npx --yes @bufbuild/buf@1.47.2 generate

# Monolith MVP does not link gRPC generated Go; keep contracts via buf, ship only OpenAPI gen in repo.
proto-prune:
	find $(ROOT)services/backend/internal/gen -mindepth 1 -maxdepth 1 ! -name openapi -exec rm -rf {} + 2>/dev/null || true

spec-copy:
	cp $(ROOT)openapi/openapi.yaml $(ROOT)services/backend/internal/spec/openapi.yaml

tidy:
	cd $(ROOT)services/backend && go mod tidy

docker-build:
	docker compose -f $(ROOT)deploy/compose/docker-compose.yml build
