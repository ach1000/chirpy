# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP server that binds to all interfaces on port 8080 and responds with a 404 Not Found status to all requests.

## Architecture

### Current Implementation
The server consists of a single `main.go` file that:

1. **Creates an http.ServeMux**: A request multiplexer (router) that routes HTTP requests
2. **Creates an http.Server struct**: Configured with:
   - `Handler`: Set to the ServeMux (though no routes are registered yet)
   - `Addr`: Set to ":8080" to bind on all interfaces on port 8080
3. **Calls ListenAndServe()**: Starts the HTTP server and blocks until shutdown

### Why 404 Always Returned
Since no route handlers are registered in the ServeMux, every request matches the catch-all 404 handler that Go's standard library provides by default.

## Building and Running

```bash
# Build the executable
go build -o out

# Run the server
./out
```

The server will start listening on `http://localhost:8080` (and all interfaces).

## Testing
Requests to any path or method return:
- **Status Code**: 404 Not Found
- **Body**: "404 page not found"

Example:
```bash
curl -v http://localhost:8080/
```

## Key Design Decisions
- **All Interfaces**: Using `:8080` instead of `localhost:8080` to bind on all interfaces, allowing testing from different machines
- **Standard Library Only**: Uses Go's `net/http` package (no external dependencies)
- **Minimal Code**: Only what's necessary to meet the requirements

## Future Changes
When handler logic is added:
1. Register handlers with `mux.HandleFunc()` or `mux.Handle()`
2. Rebuild with `go build -o out`
3. Restart the server with `./out`

---
**Note**: Update this file whenever significant changes are made to the server implementation.
