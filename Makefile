BIN_IMAGE = blockbook-build
DEB_IMAGE = blockbook-build-deb
PACKAGER = $(shell id -u):$(shell id -g)
NO_CACHE = false
UPDATE_VENDOR = 1
ARGS ?=

TARGETS=$(subst .json,, $(shell ls configs/coins))

.PHONY: build build-debug test deb

build: .bin-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src -v $(CURDIR)/build:/out $(BIN_IMAGE) make build ARGS="$(ARGS)"

build-debug: .bin-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src -v $(CURDIR)/build:/out $(BIN_IMAGE) make build-debug ARGS="$(ARGS)"

test: .bin-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src --network="host" $(BIN_IMAGE) make test ARGS="$(ARGS)"

test-all: .bin-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src --network="host" $(BIN_IMAGE) make test-all ARGS="$(ARGS)"

deb-backend-%: .deb-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src -v $(CURDIR)/build:/out $(DEB_IMAGE) /build/build-deb.sh backend $* $(ARGS)

deb-blockbook-%: .deb-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src -v $(CURDIR)/build:/out $(DEB_IMAGE) /build/build-deb.sh blockbook $* $(ARGS)

deb-%: .deb-image
	docker run -t --rm -e PACKAGER=$(PACKAGER) -e UPDATE_VENDOR=$(UPDATE_VENDOR) -v $(CURDIR):/src -v $(CURDIR)/build:/out $(DEB_IMAGE) /build/build-deb.sh all $* $(ARGS)

$(addprefix all-, $(TARGETS)): all-%: clean-deb build-images deb-%

all: clean-deb build-images $(addprefix deb-, $(TARGETS))

build-images:
	rm -f .bin-image .deb-image
	$(MAKE) .bin-image .deb-image

.bin-image:
	docker build --no-cache=$(NO_CACHE) -t $(BIN_IMAGE) build/docker/bin
	@ docker images -q $(BIN_IMAGE) > $@

.deb-image: .bin-image
	docker build --no-cache=$(NO_CACHE) -t $(DEB_IMAGE) build/docker/deb
	@ docker images -q $(DEB_IMAGE) > $@

clean: clean-bin clean-deb

clean-all: clean clean-images

clean-bin:
	find build -maxdepth 1 -type f -executable -delete

clean-deb:
	rm -rf build/pkg-defs
	rm -f build/*.deb

clean-images: clean-bin-image clean-deb-image

clean-bin-image:
	- docker rmi $(BIN_IMAGE)
	@ rm -f .bin-image

clean-deb-image:
	- docker rmi $(DEB_IMAGE)
	@ rm -f .deb-image
