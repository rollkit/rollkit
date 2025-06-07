include ./scripts/build.mk
include ./scripts/test.mk
include ./scripts/proto.mk
include ./scripts/utils.mk
include ./scripts/run.mk

# Sets the default make target to `build`.
# Requires GNU Make >= v3.81.
.DEFAULT_GOAL := build

## help: Show this help message
help: Makefile
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' | sort | sed -e 's/^/ /'
.PHONY: help
