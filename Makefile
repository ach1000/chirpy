.PHONY: build run clean test

build:
	go build -o chirpy

run: build
	./chirpy

test:
	go test ./...

clean:
	rm -f chirpy
