########################################################
#                       Makefile                       #
########################################################

# Default target
all: bindings


########################################################
#                       Building                       #
########################################################

# Target for building the application in all directories
bindings: |
	forge build --silent --extra-output-files bin --extra-output-files abi 
	go generate ./...
