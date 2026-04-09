BINARY = bin/baby-monitor

.PHONY: all build clean lint

all: build

build: $(BINARY)

$(BINARY):
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/module/...

clean:
	rm -rf bin/

lint:
	go vet ./...
