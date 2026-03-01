.PHONY: build lint clean run test

build:
	go build -o ralph-hub ./cmd/hub

lint:
	golangci-lint run

clean:
	rm -f ralph-hub

run: build
	./ralph-hub

test:
	go test ./...
