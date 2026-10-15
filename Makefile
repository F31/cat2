.PHONY: gen lint test test-integration up down archcheck

SERVER_DIR := apps/server

gen: ## 契约生成（OpenAPI → Go stub + TS 类型；事件 Schema → TS 类型）
	cd $(SERVER_DIR) && go tool oapi-codegen -config internal/gen/oapi-codegen.yaml ../../contracts/openapi/mp.yaml
	pnpm exec openapi-typescript contracts/openapi/mp.yaml -o packages/api-client/src/schema.d.ts
	mkdir -p packages/domain-types/src/events
	for f in contracts/events/*.json; do \
		n=$$(basename $$f .json); \
		pnpm exec json2ts "$$f" -o "packages/domain-types/src/events/$$n.ts"; \
	done

lint: archcheck ## 全量静态检查
	cd $(SERVER_DIR) && go vet ./...

archcheck: ## 边界检查（依赖阶梯/表前缀/infra禁令/bff只读）
	cd tools/archcheck && go run . -root ../../$(SERVER_DIR)

test: ## 后端单元测试
	cd $(SERVER_DIR) && go test ./...

test-integration: ## 后端集成测试（需 CATCUBE_TEST_DATABASE_URL 指向可用 PG；共享同一测试库，须 -p 1 串行）
	cd $(SERVER_DIR) && go test -tags=integration -p 1 -count=1 ./...

up: ## S0 一键起（app 全角色 + PostgreSQL）
	docker compose -f deploy/compose/docker-compose.s0.yml up -d --build

down:
	docker compose -f deploy/compose/docker-compose.s0.yml down
