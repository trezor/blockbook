SHELL = /bin/bash
VERSION ?= devel
GITCOMMIT ?= $(shell cd /src && git config --global --add safe.directory /src && git describe --always --dirty)
BUILDTIME = $(shell date --iso-8601=seconds)
LDFLAGS := -X github.com/trezor/blockbook/common.version=$(VERSION) -X github.com/trezor/blockbook/common.gitcommit=$(GITCOMMIT) -X github.com/trezor/blockbook/common.buildtime=$(BUILDTIME)
BLOCKBOOK_BASE := $(GOPATH)/src/github.com/trezor
BLOCKBOOK_SRC := $(BLOCKBOOK_BASE)/blockbook
ARGS ?=
# staticcheck v0.8.x needs Go 1.26; on an older image go fetches that toolchain for the run.
STATICCHECK_VERSION ?= v0.8.2
GOVULNCHECK_VERSION ?= v1.10.0

# JEMALLOC is set by the image build arg. The static archive goes in by path because cgo's linker-flag
# allowlist rejects -l:libjemalloc_pic.a; gcc resolves the multiarch directory.
ifneq ($(JEMALLOC),0)
JEMALLOC_LIB := $(shell gcc -print-file-name=libjemalloc_pic.a)
ifeq ($(JEMALLOC_LIB),libjemalloc_pic.a)
$(error libjemalloc_pic.a not found; install libjemalloc-dev or build the image with JEMALLOC=0)
endif
export CGO_LDFLAGS := $(CGO_LDFLAGS) $(JEMALLOC_LIB)
endif

all: build tools

build: prepare-sources
	cd $(BLOCKBOOK_SRC) && go build -o $(CURDIR)/blockbook -ldflags="-s -w $(LDFLAGS)" $(ARGS)
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

build-debug: prepare-sources
	cd $(BLOCKBOOK_SRC) && go build -o $(CURDIR)/blockbook -ldflags="$(LDFLAGS)" $(ARGS)
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

tools:
	cp $(CURDIR)/{ldb,sst_dump} /out
	chown $(PACKAGER) /out/{ldb,sst_dump}

test: prepare-sources
	cd $(BLOCKBOOK_SRC) && go test -tags 'unittest' `go list ./... | grep -vP '^github.com/trezor/blockbook/(contrib|tests)'` $(ARGS)

test-integration: prepare-sources
	cd $(BLOCKBOOK_SRC) && go test -tags 'integration' `go list github.com/trezor/blockbook/tests/...` -run 'TestIntegration/.*/(rpc|sync)' -timeout 30m $(ARGS)

test-e2e: prepare-sources
	@echo "Go API e2e tests were removed. Run contrib/tests/run-openapi-tests.sh from the repository root."
	@false

test-connectivity: prepare-sources
	cd $(BLOCKBOOK_SRC) && go test -tags 'integration' github.com/trezor/blockbook/tests -run "$${CONNECTIVITY_REGEX:-TestIntegration/.*/connectivity}" -timeout 3m $(ARGS)

test-all: prepare-sources
	cd $(BLOCKBOOK_SRC) && go test -tags 'unittest integration' `go list ./... | grep -v '^github.com/trezor/blockbook/contrib'` -timeout 30m $(ARGS)

# staticcheck runs only the SA (bug) checks; deprecations, style and unused code are not gated yet.
lint: prepare-sources
	cd $(BLOCKBOOK_SRC) && unformatted="$$("$$(go env GOROOT)/bin/gofmt" -l .)" && [ -z "$$unformatted" ] || (echo "gofmt needed:" && echo "$$unformatted" && false)
	cd $(BLOCKBOOK_SRC) && go mod tidy -diff
	cd $(BLOCKBOOK_SRC) && go vet -tags 'unittest integration' ./...
	cd $(BLOCKBOOK_SRC) && go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -tags 'unittest integration' -checks 'SA*,-SA1019' ./...

# Reports known vulnerabilities reachable from Blockbook code, including the image's Go toolchain.
vulncheck: prepare-sources
	cd $(BLOCKBOOK_SRC) && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) -tags 'unittest' ./... $(ARGS)

# Regenerates blockbook-api.ts from the Go structs and hands it back to the mounted checkout.
typescriptify: prepare-sources
	cd $(BLOCKBOOK_SRC) && go run ./build/tools/typescriptify
	cp $(BLOCKBOOK_SRC)/blockbook-api.ts /src/blockbook-api.ts
	chown $(PACKAGER) /src/blockbook-api.ts

prepare-sources:
	@ [ -n "`ls /src 2> /dev/null`" ] || (echo "/src doesn't exist or is empty" 1>&2 && exit 1)
	rm -rf $(BLOCKBOOK_SRC)
	mkdir -p $(BLOCKBOOK_BASE)
	cp -r /src $(BLOCKBOOK_SRC)
	cd $(BLOCKBOOK_SRC) && go mod download
