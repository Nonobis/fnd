VERSION=0.1.5

all:
	go build -o bin/fnd -ldflags "-X main.version=$(VERSION)"

docker:
	docker build -t biberino/fnd:$(VERSION) -f docker/Dockerfile .

tarball:
	docker save -o fnd-$(VERSION).tar fnd:$(VERSION)

.PHONY: all docker tar