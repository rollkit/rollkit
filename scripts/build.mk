
# Extract the latest Git tag as the version number
VERSION := $(shell git describe --tags --abbrev=0)
GITSHA := $(shell git rev-parse --short HEAD)
LDFLAGS := \
	-X github.com/rollkit/rollkit/pkg/cmd.Version=$(VERSION) \
	-X github.com/rollkit/rollkit/pkg/cmd.GitSHA=$(GITSHA)


## install: Install rollkit CLI
install:
	@echo "--> Installing Testapp CLI"
	@cd rollups/testapp && go install -ldflags "$(LDFLAGS)" .
	@echo "--> Testapp CLI Installed!"
	@echo "    Check the version with: testapp version"
	@echo "    Check the binary with: which testapp"
.PHONY: install

## build: build rollkit CLI
build:
	@echo "--> Building Testapp CLI"
	@mkdir -p $(CURDIR)/build
	@cd rollups/testapp && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/testapp .
	@echo "--> Testapp CLI Built!"
	@echo "    Check the version with: rollups/testapp version"
	@echo "    Check the binary with: $(CURDIR)/rollups/testapp"
.PHONY: build

build-da:
	@echo "--> Building local-da"
	@mkdir -p $(CURDIR)/build
	@cd da && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/local-da ./cmd/local-da
	@echo "    Check the binary with: $(CURDIR)/rollups/local-da"
.PHONY: build-da

## clean: clean and build
clean: 
	@echo "--> Cleaning Testapp CLI"
	@rm -rf $(CURDIR)/build/testapp
	@echo "--> Testapp CLI Cleaned!"
.PHONY: clean
