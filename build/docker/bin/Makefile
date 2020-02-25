SHELL = /bin/bash
UPDATE_VENDOR ?= 1
VERSION ?= devel
GITCOMMIT = $(shell cd /src && git describe --always --dirty)
BUILDTIME = $(shell date --iso-8601=seconds)
LDFLAGS := -X blockbook/common.version=$(VERSION) -X blockbook/common.gitcommit=$(GITCOMMIT) -X blockbook/common.buildtime=$(BUILDTIME)
BLOCKBOOK_SRC := $(GOPATH)/src/github.com/trezor/blockbook
ARGS ?=
TAGS = ""

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
	TAGS = 'unittest'
	echo $(CURDIR)
	cd $(BLOCKBOOK_SRC) && bash /build/run_test.sh

test-integration: prepare-sources generate-data
    TAGS = 'integration'
	echo $(CURDIR)
	cd $(BLOCKBOOK_SRC) && bash /build/run_test.sh

test-all: prepare-sources generate-data
	TAGS = 'unittest integration'
	echo $(CURDIR)
	cd $(BLOCKBOOK_SRC) && bash /build/run_test.sh

prepare-sources:
	@ [ -n "`ls /src 2> /dev/null`" ] || (echo "/src doesn't exist or is empty" 1>&2 && exit 1)
	rm -rf $(BLOCKBOOK_SRC)
	cp -r /src/github.com/trezor/$(BLOCKBOOK_SRC)
	$(MAKE) prepare-vendor

prepare-vendor:
	@ if [ "$(UPDATE_VENDOR)" -eq 1 ]; then \
		echo "Updating vendor"; \
		cd $(BLOCKBOOK_SRC) && rm -rf vendor* && cp -r /build/vendor . && dep ensure -vendor-only ; \
	else \
		echo "Update of vendor not demanded, keeping version from src" ; \
	fi

generate-data:
	cd $(BLOCKBOOK_SRC) && packr clean && packr
