.PHONY: build lint clean run test build-frontend dist

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

build: build-frontend
	go build -o ralph-hub ./cmd/hub

build-frontend:
	cd web && npm ci && npm run build
	rm -rf internal/frontend/dist
	cp -r web/out internal/frontend/dist

lint:
	golangci-lint run

clean:
	rm -f ralph-hub
	rm -rf dist/
	rm -rf internal/frontend/dist
	mkdir -p internal/frontend/dist
	touch internal/frontend/dist/.gitkeep

run: build
	./ralph-hub

test:
	go test ./...

dist: build-frontend
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -o "dist/ralph-hub-$$os-$$arch$$ext" ./cmd/hub; \
	done
	@echo "Done. Binaries in dist/"
