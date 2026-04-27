APP=loadbalancer
CMD=./cmd/loadbalancer
CONFIG?=./config.example.yaml

.PHONY: run backend-a backend-b test test-integration bench tidy fmt check

run:
	@set -e; \
	go run $(CMD) -config $(CONFIG) || { \
	code=$$?; \
	if [ $$code -ne 130 ]; then exit $$code; fi; \
	}

backend-a:
	go run ./cmd/mockbackend -name users-a -addr :9001

backend-b:
	go run ./cmd/mockbackend -name users-b -addr :9002

test:
	go test ./...

test-integration:
	go test -run TestIntegrationFailoverAndRecovery ./internal/lb

bench:
	go test -bench . -benchmem ./...

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

check: tidy fmt test
