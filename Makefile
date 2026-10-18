.PHONY: build run test fuzz bench lint tidy dashboard

# Builds the console into the Go embed directory: Vite writes straight into
# internal/proxy/inbound/dashboard/dist, which the binary embeds. build and run
# depend on it, so the embedded UI always matches web/ with no extra step.
dashboard:
	cd web && npm install --no-audit --no-fund --prefer-offline
	cd web && npm run build

# dist/ is also committed, so a bare `go build` and `make test` work without
# Node; the Dockerfile builds the console itself, in a Node stage above the Go
# builder, inside the same file.
build: dashboard
	go build -o bin/claudeproxy .

run: dashboard
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
