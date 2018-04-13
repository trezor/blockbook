build: prepare-sources
	cd $(GOPATH)/src/blockbook && go build -o $(CURDIR)/blockbook -ldflags="-s -w"
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

build-debug: prepare-sources
	cd $(GOPATH)/src/blockbook && go build -o $(CURDIR)/blockbook
	cp $(CURDIR)/blockbook /out/blockbook
	chown $(PACKAGER) /out/blockbook

test: prepare-sources
	cd $(GOPATH)/src/blockbook && go test -short ./...

test-all: prepare-sources
	cd $(GOPATH)/src/blockbook && go test ./...

prepare-sources:
	@ [ -n "`ls /src 2> /dev/null`" ] || (echo "/src doesn't exist or is empty" 1>&2 && exit 1)
	cp -r /src $(GOPATH)/src/blockbook
	rm -rf $(GOPATH)/src/blockbook/vendor
	cd $(GOPATH)/src/blockbook && dep ensure -vendor-only
