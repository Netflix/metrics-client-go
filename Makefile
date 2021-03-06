.PHONY: all testdeps install test test_race metalinter validate

all: validate

testdeps:
	go get github.com/alecthomas/gometalinter
	gometalinter --install

install:
	go install ./...

test:
	go test -bench -cover -v ./...

test_race:
	go test -bench -cover -v -race ./...

metalinter: testdeps install
	gometalinter --vendor --tests --concurrency=4 --deadline=300s --enable=unused --enable=goimports --enable=gofmt ./...

validate: test metalinter test_race
