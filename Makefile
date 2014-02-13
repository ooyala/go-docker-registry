PROJECT_ROOT := $(shell pwd)
VENDOR_PATH  := $(PROJECT_ROOT)/vendor

GOPATH := $(GOPATH):$(PROJECT_ROOT):$(VENDOR_PATH)
export GOPATH

all: build

clean:
	@rm -rf bin $(PKG_DIR) $(VENDOR_PATH)/bin $(VENDOR_PATH)/pkg

init: clean
	@mkdir bin

build: init
	@go build -o bin/registry registry.go

test:
	@echo "I don't normally test my code, but when I do, I do it on production."

fmt:
	@gofmt -l -w registry.go
	@find src -name \*.go -exec gofmt -l -w {} \;
