# Chirpy Project Memory

## Overview
- Language: Go
- Entry point: main.go
- Purpose right now: run a basic HTTP server on localhost:8080

## Current Server Behavior
- Uses net/http with a ServeMux and http.Server
- Binds to :8080
- No routes are registered yet, so / returns 404 Not Found

## Commands
- Build: go build -o out
- Run binary: ./out
- Alternative run: go run .
- Quick check: curl -i http://localhost:8080 | head -n 5
