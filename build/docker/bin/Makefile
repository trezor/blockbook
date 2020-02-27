SHELL = /bin/bash
UPDATE_VENDOR ?= 1
VERSION ?= devel
GITCOMMIT = $(shell cd /src && git describe --always --dirty)
BUILDTIME = $(shell date --iso-8601=seconds)
LDFLAGS := -X github.com/trezor/blockbook/common.version=$(VERSION) -X github.com/trezor/blockbook/common.gitcommit=$(GITCOMMIT) -X github.com/trezor/blockbook/common.buildtime=$(BUILDTIME)
BLOCKBOOK_BASE := $(GOPATH)/src/github.com/trezor
BLOCKBOOK_SRC := $(BLOCKBOOK_BASE)/blockbook
ARGS ?=

all: build tools

build: prepare-sources generate-data
	cd $(BLOCKBOOK_SRC) && go build -o $(CURDIR)/blockbook -ldflags="-s -w $(LDFLAGS)" $(ARGS)
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

build-debug: prepare-sources generate-data
	cd $(BLOCKBOOK_SRC) && go build -o $(CURDIR)/blockbook -ldflags="$(LDFLAGS)" $(ARGS)
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

tools:
	cp $(CURDIR)/{ldb,sst_dump} /out
	chown $(PACKAGER) /out/{ldb,sst_dump}

test: prepare-sources generate-data
	cd $(BLOCKBOOK_SRC) && go test -tags unittest `go list ./... | grep -vP '^github.com/trezor/blockbook/(contrib|tests)'` $(ARGS)

test-integration: prepare-sources generate-data
	cd $(BLOCKBOOK_SRC) && go test -tags integration `go list blockbook/tests/...` $(ARGS)

test-all: prepare-sources generate-data
	cd $(BLOCKBOOK_SRC) && go test -tags 'unittest integration' `go list ./... | grep -v '^github.com/trezor/blockbook/contrib'` $(ARGS)

prepare-sources:
	@ [ -n "`ls /src 2> /dev/null`" ] || (echo "/src doesn't exist or is empty" 1>&2 && exit 1)
	rm -rf $(BLOCKBOOK_SRC)
	mkdir -p $(BLOCKBOOK_BASE)
	cp -r /src $(BLOCKBOOK_SRC)
	$(MAKE) prepare-vendor

prepare-vendor:
	@ if [ "$(UPDATE_VENDOR)" -eq 1 ]; then \
		echo "Updating vendor"; \
		cd $(BLOCKBOOK_SRC) && rm -rf vendor* && cp -r /build/vendor . && env GO111MODULE=on go mod vendor ; \
	else \
		echo "Update of vendor not demanded, keeping version from src" ; \
	fi

generate-data:
	cd $(BLOCKBOOK_SRC) && packr clean && packr
