.PHONY: build deps run clean proto test

# Variables
BINARY_NAME=helios
BUILD_DIR=bin
MAIN_PATH=./cmd/helios
PROTO_SOURCE_DIR=helios-protos
PROTO_BUILD_DIR=generated

# Find all .proto files in the proto directory and subdirectories
PROTO_SRC := $(shell find $(PROTO_SOURCE_DIR) -name "*.proto")

# 1=true, 0=false
DOCKER_DISABLED=1
export DOCKER_DISABLED

MKDIR = mkdir -p $(1)
RM = rm -rf
SEPARATOR = /

# Commands
build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

	go mod download
	go mod tidy

run:
	go run $(MAIN_PATH)	

clean:
	rm -rf $(BUILD_DIR)
	go clean

proto:
	$(call MKDIR,$(PROTO_BUILD_DIR))

	protoc -I=$(PROTO_SOURCE_DIR) --go_out=$(PROTO_BUILD_DIR) $(PROTO_SRC)

test:
	go test ./... -v
	
submodule:
	git submodule update --remote --merge