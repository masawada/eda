.PHONY: build test vet fmt install clean

build:
	go build -o eda .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

install:
	go install .

clean:
	rm -f eda
