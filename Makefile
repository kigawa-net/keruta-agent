.PHONY: generate-client clean build test

# Generate OpenAPI client
generate-client:
	@echo "Generating OpenAPI client for Go..."
	@mkdir -p internal/client
	@go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
	@oapi-codegen -config client-config.yaml http://localhost:8080/v3/api-docs > internal/client/client.go

# Clean generated files
clean:
	rm -rf internal/client/

# Build the application
build:
	go build -o bin/keruta-agent cmd/main.go

# Run tests
test:
	go test ./...

# Install dependencies
deps:
	go mod download
	go mod tidy