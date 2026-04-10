BINARY = bin/baby-monitor

.PHONY: all build clean lint

all: build

build: $(BINARY)

$(BINARY):
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/module/...

module.tar.gz: $(BINARY)
	tar -czf module.tar.gz meta.json bin/baby-monitor

clean:
	rm -rf bin/ module.tar.gz

lint:
	go vet ./...
