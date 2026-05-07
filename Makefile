.PHONY: dev build clean

dev:
	@echo "Starting Mulwiki..."
	@echo "Go backend: http://localhost:8080"
	@echo "Web frontend: http://localhost:3000"
	@echo ""
	cd server && go run ./cmd/server &
	cd apps/web && pnpm dev

build:
	cd server && go build -o bin/server ./cmd/server
	cd server && go build -o bin/mulwiki ./cmd/mulwiki
	cd apps/web && pnpm build

clean:
	rm -rf server/bin
	rm -rf apps/web/.next
