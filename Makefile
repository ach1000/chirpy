.PHONY: build run

build:
	go build -o chirpy

run: build
	./chirpy
