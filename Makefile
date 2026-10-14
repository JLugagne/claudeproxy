.PHONY: build run test fuzz bench lint tidy

build:
	go build -o bin/claudeproxy .

run:
	go run . serve -c configs/config.yaml

test:
	go test -race ./... -count=1

fuzz:
	go test ./internal/pkg/jsonscan -run '^$$' -fuzz FuzzTopLevelString -fuzztime 30s

bench:
	go test ./internal/pkg/jsonscan -run '^$$' -bench . -benchmem

lint:
	go vet ./...
	go-arch-lint check

tidy:
	go mod tidy
