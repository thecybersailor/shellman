.PHONY: help test fmt vet lint

help:
	@echo "CLI Service Commands:"
	@echo "  make test          - Run tests"
	@echo "  make fmt           - Run go fmt"
	@echo "  make vet           - Run go vet"
	@echo "  make lint          - Run golangci-lint"

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed."; \
	fi
