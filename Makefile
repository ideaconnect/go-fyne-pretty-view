# Makefile for go-fyne-pretty-view

GO       ?= go
PKG      := ./...
DEMO_DIR := ./cmd/prettyview-demo
BIN_DIR  := bin
DEMO_BIN := $(BIN_DIR)/prettyview-demo

# Overridable knobs:
#   make run FILE=testdata/catalog.xml
#   make test RUN=TestSearchPlain
#   make bench BENCH=BenchmarkParse
#   make shots                      (writes PNGs to /tmp and docs/)
FILE  ?= testdata/openapi.json
RUN   ?= .
BENCH ?= .
COUNT ?= 1

.DEFAULT_GOAL := help

## help: show this help
.PHONY: help
help:
	@echo "go-fyne-pretty-view — make targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

## build: compile the library and the demo binary into ./bin
.PHONY: build
build:
	$(GO) build $(PKG)
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(DEMO_BIN) $(DEMO_DIR)
	@echo "built $(DEMO_BIN)"

## run: build and run the demo (override the file with FILE=path)
.PHONY: run
run:
	$(GO) run $(DEMO_DIR) $(FILE)

## test: run the test suite (filter with RUN=TestName)
.PHONY: test
test:
	$(GO) test -run '$(RUN)' -count=$(COUNT) $(PKG)

## race: run the test suite with the race detector
.PHONY: race
race:
	$(GO) test -race -count=$(COUNT) $(PKG)

## cover: run tests and open a coverage report
.PHONY: cover
cover:
	$(GO) test -coverprofile=coverage.out $(PKG)
	$(GO) tool cover -func=coverage.out | tail -1

## bench: run benchmarks (filter with BENCH=BenchmarkName)
.PHONY: bench
bench:
	$(GO) test -run '^$$' -bench='$(BENCH)' -benchmem $(PKG)

## shots: render screenshots to /tmp and docs/ (needs no display)
.PHONY: shots
shots:
	PV_SHOTS=1 $(GO) test -run TestCaptureScreenshots $(PKG)

## vet: run go vet
.PHONY: vet
vet:
	$(GO) vet $(PKG)

## fmt: format all Go sources
.PHONY: fmt
fmt:
	$(GO) fmt $(PKG)

## tidy: tidy go.mod / go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy

## check: fmt-check, vet, and race test (CI gate)
.PHONY: check
check: vet
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "gofmt needed (run 'make fmt')"; exit 1)
	$(GO) test -race $(PKG)

## clean: remove build and coverage artifacts
.PHONY: clean
clean:
	$(GO) clean
	rm -rf $(BIN_DIR) coverage.out
