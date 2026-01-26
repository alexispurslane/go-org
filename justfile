# Default target
default:
    @just test

# Run tests
test:
    go get -d -t ./...
    go test ./... -v

# Check if go is installed
setup:
    @!/usr/bin/env -S command -v go >/dev/null 2>&1 || (echo "go not installed" && exit 1)

# Run fuzz testing
fuzz:
    @echo "also see \"http://lcamtuf.coredump.cx/afl/README.txt\""
    go get github.com/dvyukov/go-fuzz/go-fuzz
    go get github.com/dvyukov/go-fuzz/go-fuzz-build
    mkdir -p fuzz fuzz/corpus
    cp org/testdata/*.org fuzz/corpus/
    go-fuzz-build github.com/alexispurslane/go-org/org
    go-fuzz -bin=./org-fuzz.zip -workdir=fuzz
