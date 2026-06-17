# Chirpy Project Memory

## Overview
- Language: Go
- Entry point: chirpy.go
- Purpose right now: run a basic HTTP server on localhost:8080

## Current Server Behavior
- Uses net/http with a ServeMux and http.Server
- Binds to :8080
- "/app/" is registered with http.StripPrefix("/app", http.FileServer(http.Dir("."))), serving index.html from the project root (e.g. /app/ -> index.html)
- "/healthz" is registered via mux.HandleFunc(handlerReadiness); responds 200 OK, Content-Type: text/plain; charset=utf-8, body "OK", for any HTTP method

## Commands
- Build: go build -o out
- Run binary: ./out
- Alternative run: go run .
- Quick check: curl -i http://localhost:8080 | head -n 5
- Clean: make clean (removes built chirpy binary)
