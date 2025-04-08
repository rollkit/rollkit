# Makefile for attester

# Variables
PROTOC = protoc
PROTO_DIR = proto
PROTO_SRC = $(PROTO_DIR)/attester/v1/attester.proto
API_GEN_DIR = api/gen

# Go Protobuf output directory needs to match the Go module structure relative to API_GEN_DIR
# Since the proto package is attester.v1, the output dir will be api/gen/attester/v1
GO_OUT_DIR = $(API_GEN_DIR)
GRPC_OUT_DIR = $(API_GEN_DIR)
GO_PROTO_GENERATED_DIR = $(API_GEN_DIR)/attester/v1

# Docker settings
DOCKER_PROTO_IMAGE = namely/protoc:latest # Using latest tag
# Use shell for id -u/-g to get current user/group IDs
CURRENT_UID := $(shell id -u)
CURRENT_GID := $(shell id -g)

# Add docker-proto-gen to .PHONY
.PHONY: proto-gen clean-protos docker-proto-gen test

# Target to generate Go code from .proto files using local tools
proto-gen: $(GO_PROTO_GENERATED_DIR)/attester.pb.go $(GO_PROTO_GENERATED_DIR)/attester_grpc.pb.go

$(GO_PROTO_GENERATED_DIR)/attester.pb.go $(GO_PROTO_GENERATED_DIR)/attester_grpc.pb.go: $(PROTO_SRC)
	@echo "Generating Go code from $(PROTO_SRC) using local tools..."
	@mkdir -p $(GO_PROTO_GENERATED_DIR)
	$(PROTOC) -I $(PROTO_DIR) --go_out=$(GO_OUT_DIR) --go_opt=paths=source_relative \
    --go-grpc_out=$(GRPC_OUT_DIR) --go-grpc_opt=paths=source_relative \
    $(PROTO_SRC)

# Target to generate Go code from .proto files using Docker
docker-proto-gen:
	@echo "Generating Go code from $(PROTO_SRC) using Docker ($(DOCKER_PROTO_IMAGE))..."
	@mkdir -p $(GO_PROTO_GENERATED_DIR) # Ensure output directory exists
	@docker run --rm \
		-v $(shell pwd):/workspace \
		-w /workspace \
		--user $(CURRENT_UID):$(CURRENT_GID) \
		$(DOCKER_PROTO_IMAGE) \
		-I $(PROTO_DIR) \
		--go_out=$(API_GEN_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(GRPC_OUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_SRC)
	@echo "Docker proto generation complete."

# Target to remove generated Go code
clean-protos:
	@echo "Cleaning generated proto files..."
	@rm -f $(GO_PROTO_GENERATED_DIR)/attester.pb.go
	@rm -f $(GO_PROTO_GENERATED_DIR)/attester_grpc.pb.go
	@echo "Done."

.PHONY: test
test:
	@echo "Running attester tests..."
	go test ./... 