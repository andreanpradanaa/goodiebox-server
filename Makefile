.PHONY: run build test tidy compose-up compose-down

compose-up:
	docker compose up -d db

compose-down:
	docker compose down

run: compose-up
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy
