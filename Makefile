.PHONY: build run clean test reset

build:
	go build -o chirpy

run: build
	./chirpy

test:
	go test ./...

clean:
	rm -f chirpy

reset:
	cd sql/schema && goose postgres postgres://postgres:postgres@localhost:5432/chirpy down || true
	cd sql/schema && goose postgres postgres://postgres:postgres@localhost:5432/chirpy up
