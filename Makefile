.PHONY: build test clean docker-build docker-run help

APP_NAME = go-bittorrent
DOCKER_IMAGE = $(APP_NAME):latest

help:
	@echo "Available commands:"
	@echo "  make build         - Build the Go binary locally"
	@echo "  make test          - Run all Go unit tests"
	@echo "  make clean         - Remove the local binary and any downloaded .iso files"
	@echo "  make docker-build  - Build the Docker image"
	@echo "  make docker-run    - Run the Docker image (Requires TORRENT and OUT variables)"
	@echo "                       Example: make docker-run TORRENT=debian.torrent OUT=debian.iso"

build:
	go build -o $(APP_NAME) main.go

test:
	go test ./...

clean:
	rm -f $(APP_NAME)
	rm -f *.iso
	rm -f *.tar
	rm -f *.img

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run:
	@if [ -z "$(TORRENT)" ] || [ -z "$(OUT)" ]; then \
		echo "Error: Need to provide TORRENT and OUT variables."; \
		echo "Example: make docker-run TORRENT=debian.torrent OUT=debian.iso"; \
		exit 1; \
	fi
	@echo "Running $(DOCKER_IMAGE) with volume mount..."
	docker run --rm -v $(PWD):/data $(DOCKER_IMAGE) /data/$(TORRENT) /data/$(OUT)
