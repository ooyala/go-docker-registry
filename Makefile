PROJECT_ROOT := $(shell pwd)
VENDOR_PATH  := $(PROJECT_ROOT)/vendor

GOPATH := $(GOPATH):$(PROJECT_ROOT):$(VENDOR_PATH)
export GOPATH

PKGS := github.com/cespare/go-apachelog
PKGS += github.com/crowdmob/goamz/aws
PKGS += github.com/crowdmob/goamz/s3

all: build

clean:
	@rm -rf bin pkg $(VENDOR_PATH)/bin $(VENDOR_PATH)/pkg

init: clean
	@mkdir bin

build: init pkg
	@go build -o bin/registry registry.go

.PHONY: pkg
pkg:
	for p in $(PKGS); do go install $$p; done

test: clean
ifdef TEST_PACKAGE
	@echo "Testing $$TEST_PACKAGE..."
	@go test -cover $$TEST_FLAGS $$TEST_PACKAGE
else
	@for p in `find ./src -type f -name "*.go" |sed 's-\./src/\(.*\)/.*-\1-' |sort -u`; do \
		echo "Testing $$p..."; \
		go test -cover $$TEST_FLAGS $$p || exit 1; \
	done
	@echo
	@echo "ok."
endif

annotate:
ifdef TEST_PACKAGE
	@echo "Annotating $$TEST_PACKAGE..."
	@go test -coverprofile=cover.out $$TEST_FLAGS $$TEST_PACKAGE
	@go tool cover -html=cover.out
	@rm -f cover.out
else
	@echo "Specify package!"
endif

fmt:
	@gofmt -l -w registry.go
	@find src -name \*.go -exec gofmt -l -w {} \;
