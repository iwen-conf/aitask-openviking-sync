VERSION ?= v0.1.0
DOCKER_USER ?= iluwenconf

.PHONY: build install release clean

build:
	@bash scripts/release.sh $(VERSION) $(DOCKER_USER)

install: build
	@echo "(install is part of release; build target above already calls 'lzc-cli app install')"

release:
	@bash scripts/release.sh $(VERSION) $(DOCKER_USER)

clean:
	rm -f aitask.lpk .lzc-manifest.rendered.yml
	rm -rf content
