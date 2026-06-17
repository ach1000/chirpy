.PHONY: build run clean

build:
	go build -o chirpy

run: build
	./chirpy

clean:
	rm -f chirpy
