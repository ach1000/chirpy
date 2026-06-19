# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to port 8080 and serves static files from the current directory.

## Architecture

### Current Implementation
The server is implemented in `chirpy.go` with the following components:

1. **apiConfig struct**: Holds in-memory server state
   - `fileserverHits atomic.Int32`: count of requests served through the `/app/` handler, safe for concurrent access

2. **main()** function: Sets up and starts the server
   - Creates an `apiConfig` instance (`apiCfg`)
   - Creates an http.ServeMux (request multiplexer/router)
   - Registers handlers in this order:
     - `GET /healthz`: Readiness endpoint that returns 200 OK with "OK" message
     - `/app/` path: Serves files from the current directory (`.`) via `http.StripPrefix` and `http.FileServer`, wrapped in `apiCfg.middlewareMetricsInc`
     - `GET /metrics`: Returns the current hit count as plain text
     - `POST /reset`: Resets the hit count to 0
   - Creates an http.Server struct configured with:
     - `Handler`: Set to the mux
     - `Addr`: Set to ":8080"
   - Calls `ListenAndServe()` to start the HTTP server

3. **handlerReadiness()** function: Handles health check requests
   - Sets Content-Type header to "text/plain; charset=utf-8"
   - Writes HTTP 200 OK status code
   - Writes response body "OK"
   - Used for checking if the server is ready to receive traffic
   - Only registered for GET; other methods get an automatic 405

4. **middlewareMetricsInc()** method on `*apiConfig`: wraps an `http.Handler` and increments `fileserverHits` (via `.Add(1)`) on every request before calling the wrapped handler

5. **handlerMetrics()** method on `*apiConfig`: writes `Hits: x` as plain text, where `x` is the current `fileserverHits` value (via `.Load()`)

6. **handlerReset()** method on `*apiConfig`: resets `fileserverHits` to 0 (via `.Store(0)`) and returns 200 OK

### FileServer Behavior
**App Handler** (`/app/`):
- Serves files from the current directory (`.`)
- Uses `http.StripPrefix` to remove `/app/` prefix from request path
- Automatically serves `index.html` when accessing `/app/`
- Example: Request to `/app/index.html` serves `index.html`

## Building and Running

```bash
# Build the executable
make build      # or: go build -o chirpy

# Run the server
make run        # builds then runs ./chirpy

# Clean the built binary
make clean
```

The server will start listening on `http://localhost:8080`.

## Testing
No automated tests exist yet (`chirpy_test.go` not present).

### Manual Testing
- **Health Check** (`GET /healthz`): Returns 200 OK with "OK" message; other methods get 405
- **App Path** (`/app/`): Serves index.html and other files from the current directory; each hit increments the metrics counter
- **Metrics** (`GET /metrics`): Returns `Hits: x` as plain text; other methods get 405
- **Reset** (`POST /reset`): Resets the hit counter to 0; other methods get 405

Example:
```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/app/
curl http://localhost:8080/metrics
curl -X POST http://localhost:8080/reset
```

## Key Design Decisions
- **Health Check Endpoint**: A dedicated `/healthz` readiness endpoint allows external systems (load balancers, orchestration systems) to monitor server health
- **Application Server Path**: The fileserver is under `/app/` instead of `/` to avoid conflicts with the health check endpoint and future API endpoints
- **http.StripPrefix**: Used to cleanly map the `/app/` URL path to the filesystem root (e.g., `/app/index.html` → `index.html`)
- **FileServer Handler**: Uses Go's standard `http.FileServer` to serve static files without custom route logic
- **Standard Library Only**: Uses only Go's `net/http` package (no external dependencies)
- **Method-Specific Routing**: `/healthz` and `/metrics` are registered as `GET`, and `/reset` as `POST`, using Go 1.22+'s `"METHOD /path"` mux pattern syntax — mismatched methods get an automatic 405 Method Not Allowed

## Project Structure
```
chirpy/
├── chirpy.go       # Server implementation (main + handlerReadiness)
├── go.mod          # Go module definition
├── index.html      # Static HTML file served at /app/
├── Makefile        # build/run/clean targets
├── PROJECT.md       # This documentation file
└── chirpy          # Compiled binary (not tracked in git)
```

## Missing Functionality (compared to ../chirpy-old)
The sibling project `../chirpy-old` has implemented more functionality that this project lacks. To bring this project to parity, still need to add:
- **/assets/** path: serves files (e.g. a logo) from an `assets/` directory via `http.StripPrefix` + `http.FileServer`
- **makeHandler()** extraction: pull mux/handler setup out of `main()` into its own function so it's testable independently of starting a real server
- **Unit tests** (`chirpy_test.go`) covering the app/index.html handler, the assets handler, and the readiness handler

## Future Changes
When adding more static files or modifying the server:
1. Add HTML/CSS/JS files to the project root directory (or `assets/` once added)
2. Rebuild with `make build`
3. Restart the server with `make run`

The FileServer will automatically serve any files placed in the served directory.

---
**Note**: Update this file whenever significant changes are made to the server implementation.
