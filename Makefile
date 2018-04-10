PACKAGER = $(shell id -u):$(shell id -g)

.PHONY: build test deb

build:
	docker run -t --rm -e PACKAGER=$(PACKAGER) -v $(CURDIR):/src -v $(CURDIR)/build:/out blockbook-build make build
	strip build/blockbook

test:
	docker run -t --rm -e PACKAGER=$(PACKAGER) -v $(CURDIR):/src blockbook-build make test

deb:
	docker run -t --rm -e PACKAGER=$(PACKAGER) -v $(CURDIR):/src -v $(CURDIR)/build:/out blockbook-build-deb

clean:
	rm -f build/blockbook
	rm -f build/*.deb
