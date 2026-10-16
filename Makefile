.PHONY: gen lint test test-integration up down archcheck logscan install build-web typecheck dev-admin dev-miniapp perf-seed loadtest

SERVER_DIR := apps/server

gen: ## 契约生成（OpenAPI → Go stub + TS 类型；事件 Schema → TS 类型）
	cd $(SERVER_DIR) && go tool oapi-codegen -config internal/gen/oapi-codegen.yaml ../../contracts/openapi/mp.yaml
	cd $(SERVER_DIR) && go tool oapi-codegen -config internal/gen/oapi-codegen.admin.yaml ../../contracts/openapi/admin.yaml
	pnpm exec openapi-typescript contracts/openapi/mp.yaml -o packages/api-client/src/schema.d.ts
	pnpm exec openapi-typescript contracts/openapi/admin.yaml -o packages/api-client/src/admin-schema.d.ts
	mkdir -p packages/domain-types/src/events
	for f in contracts/events/*.json; do \
		n=$$(basename $$f .json); \
		pnpm exec json2ts "$$f" -o "packages/domain-types/src/events/$$n.ts"; \
	done

lint: archcheck logscan ## 全量静态检查
	cd $(SERVER_DIR) && go vet ./...

logscan: ## 日志敏感信息扫描（session_key/手机号/证件号不入日志）
	cd tools/logscan && go run . -root ../../$(SERVER_DIR)

archcheck: ## 边界检查（依赖阶梯/表前缀/infra禁令/bff只读）
	cd tools/archcheck && go run . -root ../../$(SERVER_DIR)

test: ## 后端单元测试
	cd $(SERVER_DIR) && go test ./...

test-integration: ## 后端集成测试（需 CATCUBE_TEST_DATABASE_URL 指向可用 PG；共享同一测试库，须 -p 1 串行）
	cd $(SERVER_DIR) && go test -tags=integration -p 1 -count=1 ./...

install: ## 安装前端依赖（pnpm workspace）
	pnpm install

typecheck: ## 前端类型检查（api-client + admin-web + miniapp）
	pnpm --filter @catcube/api-client build
	pnpm --filter admin-web typecheck
	pnpm --filter miniapp typecheck

build-web: ## 前端构建（后台产物 + 小程序 weapp 产物）
	pnpm --filter admin-web build
	pnpm --filter miniapp build:weapp

dev-admin: ## 本地联调：后台开发服务器（Vite，/api 代理到 127.0.0.1:8080）
	pnpm --filter admin-web dev

dev-miniapp: ## 本地联调：小程序 watch 构建（微信开发者工具导入 apps/miniapp/dist）
	pnpm --filter miniapp dev:weapp

up: ## S0 一键起（app 全角色 + PostgreSQL）
	docker compose -f deploy/compose/docker-compose.s0.yml up -d --build

down:
	docker compose -f deploy/compose/docker-compose.s0.yml down

BASE ?= http://127.0.0.1:8080
TOKEN ?=
REF ?=
C ?= 50
D ?= 20s
SCENARIOS ?= feed,search,detail

perf-seed: ## 压测造数（需 CATCUBE_PERF_DATABASE_URL 指向独立压测库）
	./tests/perf/seed.sh

loadtest: ## 轻量压测（BASE/TOKEN/REF/C/D/SCENARIOS 可覆盖；门槛不达标退出码非 0）
	cd tools/loadtest && go run . -base $(BASE) -token "$(TOKEN)" -ref "$(REF)" -c $(C) -d $(D) -scenarios $(SCENARIOS)
