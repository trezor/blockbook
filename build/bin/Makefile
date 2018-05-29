SHELL = /bin/bash
UPDATE_VENDOR ?= 1
VERSION ?= devel
GITCOMMIT = $(shell cd /src && git describe --tags --always --dirty)
BUILDTIME = $(shell date --iso-8601=seconds)
LDFLAGS := -X blockbook/common.version=$(VERSION) -X blockbook/common.gitcommit=$(GITCOMMIT) -X blockbook/common.buildtime=$(BUILDTIME)

all: build tools

build: prepare-sources
	cd $(GOPATH)/src/blockbook && go build -o $(CURDIR)/blockbook -ldflags="-s -w $(LDFLAGS)"
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

build-debug: prepare-sources
	cd $(GOPATH)/src/blockbook && go build -o $(CURDIR)/blockbook -ldflags="$(LDFLAGS)"
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

tools:
	cp $(CURDIR)/{ldb,sst_dump} /out
	chown $(PACKAGER) /out/{ldb,sst_dump}

test: prepare-sources
	cd $(GOPATH)/src/blockbook && go test -short ./...

test-all: prepare-sources
	cd $(GOPATH)/src/blockbook && go test ./...

prepare-sources:
	@ [ -n "`ls /src 2> /dev/null`" ] || (echo "/src doesn't exist or is empty" 1>&2 && exit 1)
	[ -d $(GOPATH)/src/blockbook ] || cp -r /src $(GOPATH)/src/blockbook
	$(MAKE) prepare-vendor

prepare-vendor:
	@ if [ "$(UPDATE_VENDOR)" -eq 1 ]; then \
		echo "Updating vendor"; \
		rm -rf $(GOPATH)/src/blockbook/vendor && cd $(GOPATH)/src/blockbook && dep ensure -vendor-only ; \
	else \
		echo "Update of vendor not demanded, keeping version from src" ; \
	fi
