include ./scripts/build.mk
include ./scripts/test.mk
include ./scripts/proto.mk
include ./scripts/utils.mk

## help: Show this help message
help: Makefile
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' | sort | sed -e 's/^/ /'
.PHONY: help
