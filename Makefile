.PHONY: test check build demo

test:
	go test ./...

check:
	go vet ./...
	go test -race -count=1 ./...

build:
	go build -buildvcs=false -o bin/dockline ./cmd/dockline

demo: build
	./bin/dockline import --input examples/events.ndjson --output out/events
	./bin/dockline verify --snapshot out/events --input examples/events.ndjson
