VERSION=0.1.5

all:
	go build -o bin/fnd -ldflags "-X main.version=$(VERSION)"

docker:
	docker build -t biberino/fnd:$(VERSION) -f docker/Dockerfile .

tarball:
	docker save -o fnd-$(VERSION).tar fnd:$(VERSION)

# Development commands
dev-build:
	go build -o bin/fnd -ldflags "-X main.version=$(VERSION)"

dev-up:
	docker-compose up --build

dev-down:
	docker-compose down

dev-logs:
	docker-compose logs -f

.PHONY: all docker tar dev-build dev-up dev-down dev-logs