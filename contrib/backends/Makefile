TARGETS = bitcoin zcash bcash ethereum bgold dash litecoin dogecoin vertcoin namecoin
IMAGE = blockbook-backend-build-deb
NO_CACHE = false

.PHONY: $(TARGETS)

all: $(TARGETS)

$(TARGETS): .docker-image
	docker run -t --rm -e PACKAGER="`id -u`:`id -g`" -v $(CURDIR)/$@:/deb/$@ $(IMAGE) $@
	mv $@/*.deb .

build-image:
	rm -f .docker-image
	$(MAKE) .docker-image

.docker-image:
	docker build --no-cache=$(NO_CACHE) -t $(IMAGE) docker
	@ docker images -q $(IMAGE) > $@

clean: clean-packages clean-image

clean-packages:
	rm -f *.deb

clean-image:
	- docker rmi $(IMAGE)
	@ rm -f .docker-image
