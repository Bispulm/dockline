.PHONY: build test check fmt fmt-check demo bench fuzz

build:
	go build -trimpath -buildvcs=false -o bin/dockline ./cmd/dockline

test:
	go test -count=1 ./...

fmt:
	gofmt -w cmd internal

fmt-check:
	@if test -n "$$(gofmt -l cmd internal)"; then gofmt -l cmd internal; exit 1; fi

check: fmt-check
	go vet ./...
	go test -race -count=1 ./...

demo:
	./scripts/demo.sh

bench:
	go test ./internal/engine -run '^$$' -bench BenchmarkImport -benchmem

fuzz:
	go test ./internal/model -run '^$$' -fuzz FuzzParseRecord -fuzztime 3s
