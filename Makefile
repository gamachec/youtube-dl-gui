export PATH := $(HOME)/.local/go/bin:$(HOME)/go/bin:$(PATH)

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
DIST    := dist/youtube-dl-gui-$(VERSION)-win64.zip

.PHONY: test vet build dist clean

test:
	go test ./...

vet:
	go vet ./...

build:
	wails build -platform windows/amd64 -clean -s -trimpath -ldflags "-s -w -X main.version=$(VERSION)"

# Zip containing only the exe and LISEZMOI.txt; external tools are downloaded on first run.
dist: build
	@mkdir -p dist
	@rm -f $(DIST)
	@rm -rf dist/stage && mkdir -p dist/stage
	@cp build/bin/youtube-dl-gui.exe build/dist/LISEZMOI.txt dist/stage/
	@cd dist/stage && python3 -m zipfile -c ../../$(DIST) youtube-dl-gui.exe LISEZMOI.txt
	@rm -rf dist/stage
	@ls -l $(DIST)

clean:
	rm -rf dist build/bin
