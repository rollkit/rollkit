# Generate mocks
[group('codegen')]
mock-gen:
    @echo "-> Generating mocks"
    go run github.com/vektra/mockery/v3@v3.8.1

# Install dependencies and tidy all modules
[group('codegen')]
deps:
    @echo "--> Installing dependencies"
    @go mod download
    @go mod tidy
    @go run scripts/tidy.go

# Tidy all go.mod files
[group('codegen')]
tidy-all:
    @go run -tags=tidy scripts/tidy.go
