GO_TEST_CMD ?= go test -cover -bench

.PHONY: all testdeps install test test_race metalinter validate

all: validate

testdeps:
	go get github.com/alecthomas/gometalinter
	gometalinter --install

install:
	go install ./...

test:
	$(GO_TEST_CMD) -v ./...

test_race:
	$(GO_TEST_CMD) -v -race ./...

metalinter: testdeps install
	gometalinter --vendor --tests --concurrency=4 --deadline=300s --enable=unused --enable=goimports --enable=gofmt ./...

validate: test metalinter test_race
