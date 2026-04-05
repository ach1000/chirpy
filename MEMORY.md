# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to all interfaces on port 8080 and serves static files from the current directory.

## Architecture

### Current Implementation
The server is implemented in `chirpy.go` with the following components:

1. **apiConfig** struct: Holds in-memory server state
   - `fileserverHits atomic.Int32`: Thread-safe counter for `/app/` requests

2. **makeHandler()** function: Creates and returns the HTTP handler
   - Creates an http.ServeMux (request multiplexer/router)
   - Instantiates an `apiConfig` to hold shared state
   - Registers handlers in this order:
     - `/healthz` path: Readiness endpoint that returns 200 OK with "OK" message
     - `/metrics` path: Returns hit count as plain text (`Hits: x`)
     - `/reset` path: Resets `fileserverHits` counter to 0
     - `/assets/` path: Serves files from the `assets/` directory using `http.StripPrefix` and `http.FileServer`
     - `/app/` path: Serves files from the current directory (`.`), wrapped in `middlewareMetricsInc`
   - Returns the configured handler (testable and reusable)

3. **main()** function: Sets up and starts the server
   - Creates an http.Server struct configured with:
     - `Handler`: Set to the result of `makeHandler()`
     - `Addr`: Set to ":8080" to bind on all interfaces on port 8080
   - Calls `ListenAndServe()` to start the HTTP server

4. **readinessHandler()** function: Handles health check requests
   - Sets Content-Type header to "text/plain; charset=utf-8"
   - Writes HTTP 200 OK status code
   - Writes response body "OK"
   - Used for checking if the server is ready to receive traffic

5. **middlewareMetricsInc()** method on `*apiConfig`: Middleware that increments `fileserverHits` on every request, then delegates to the next handler

### FileServer Behavior
The server uses multiple handlers for different paths:

**Readiness Handler** (`/healthz`):
- Health check endpoint for external systems
- Returns HTTP 200 OK status code
- Content-Type: `text/plain; charset=utf-8`
- Response body: `OK`
- Can be enhanced later to return 503 Service Unavailable if server is not ready

**Metrics Handler** (`/metrics`):
- Returns the number of requests served by the `/app/` file server since last restart
- Content-Type: `text/plain; charset=utf-8`
- Response body: `Hits: x` where x is the count

**Reset Handler** (`/reset`):
- Resets the `fileserverHits` counter to 0
- Returns HTTP 200 OK with no body

**Assets Handler** (`/assets/`):
- Serves files from the `assets/` directory
- Uses `http.StripPrefix` to remove `/assets/` prefix from request path
- Example: Request to `/assets/logo.png` serves `assets/logo.png`
- Returns appropriate Content-Type for served files (e.g., `image/png`)

**App Handler** (`/app/`):
- Serves files from the current directory (`.`)
- Uses `http.StripPrefix` to remove `/app/` prefix from request path
- Automatically serves `index.html` when accessing `/app/`
- Example: Request to `/app/index.html` serves `index.html`
- Returns appropriate HTTP status codes and Content-Type headers
- Supports directory listing and file downloads

## Building and Running

```bash
# Build the executable
go build -o out

# Run the server
./out
```

The server will start listening on `http://localhost:8080` (and all interfaces).

## Testing
Unit tests are provided in `chirpy_test.go`:

- **TestServeIndexHTML**: Tests that the handler serves `index.html` from `/app/` path correctly
  - Verifies the server returns a 200 status code for `/app/index.html`
  - Confirms the response contains "Welcome to Chirpy"
  - Uses the actual `makeHandler()` function from the production code

- **TestServeLogo**: Tests that the handler serves assets correctly
  - Verifies the server returns a 200 status code for `/assets/logo.png`
  - Confirms the Content-Type header indicates an image
  - Verifies the response body contains image file content

- **TestMetricsEndpoint**: Tests the `/metrics` handler
  - Sends two requests to `/app/index.html` (via a no-redirect client), then checks `/metrics` returns `Hits: 2`

- **TestResetEndpoint**: Tests the `/reset` handler
  - Sends a request to `/app/index.html`, then hits `/reset`, then verifies `/metrics` returns `Hits: 0`

- **TestReadinessEndpoint**: Tests the readiness health check endpoint
  - Verifies the server returns a 200 status code for `/healthz`
  - Confirms the Content-Type header is `text/plain; charset=utf-8`
  - Verifies the response body contains exactly "OK"

Run tests with:
```bash
go test -v
```

### Manual Testing
- **Health Check** (`/healthz`): Returns 200 OK with "OK" message
- **Metrics** (`/metrics`): Returns `Hits: x` showing number of `/app/` requests since last restart
- **Reset** (`/reset`): Resets the hit counter to 0
- **App Path** (`/app/`): Serves index.html and other files from the current directory
- **Assets Path** (`/assets/`)**: Serves files from the `assets/` directory
- **Logo**: Access at `http://localhost:8080/assets/logo.png`

Example:
```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/app/index.html
curl http://localhost:8080/assets/logo.png
curl http://localhost:8080/metrics
curl http://localhost:8080/reset
```

## Key Design Decisions
- **Testable Handler**: The `makeHandler()` function is extracted to allow unit testing of the handler logic without starting a full server
- **Health Check Endpoint**: A dedicated `/healthz` readiness endpoint allows external systems (load balancers, orchestration systems) to monitor server health
- **Explicit Path Routing**: Specific paths (`/healthz`, `/assets/`, `/app/`) ensure no conflicts and clear separation of concerns
- **Asset Directory Structure**: Assets are organized in a dedicated `assets/` directory, separated from root-level files
- **Application Server Path**: The fileserver is now under `/app/` instead of `/` to avoid conflicts with the health check endpoint and future API endpoints
- **http.StripPrefix**: Uses `http.StripPrefix` to cleanly map URL paths to filesystem directories (e.g., `/app/index.html` → `index.html`, `/assets/logo.png` → `assets/logo.png`)
- **All Interfaces**: Using `:8080` instead of `localhost:8080` to bind on all interfaces, allowing testing from different machines
- **FileServer Handler**: Uses Go's standard `http.FileServer` to serve static files without custom route logic
- **Standard Library Only**: Uses only Go's `net/http` package (no external dependencies)
- **No-redirect test client**: `http.FileServer` redirects requests for `index.html` to `./` (the directory); the default `http.Get` follows this redirect, causing the middleware to fire twice per request. Tests that check hit counts use a custom client with `CheckRedirect: http.ErrUseLastResponse` to suppress this.

## Project Structure
```
chirpy/
├── chirpy.go       # Server implementation with makeHandler() function
├── chirpy_test.go  # Unit tests for the server
├── go.mod          # Go module definition
├── index.html      # Static HTML file served at root
├── assets/         # Directory for static assets
│   └── logo.png    # Chirpy logo image
├── MEMORY.md       # This documentation file
└── out             # Compiled binary (not tracked in git)
```

## Future Changes
When adding more static files or modifying the server:
1. Add HTML/CSS/JS files to the project root directory
2. Rebuild with `go build -o out`
3. Restart the server with `./out`

The FileServer will automatically serve any files placed in the project directory.

---
**Note**: Update this file whenever significant changes are made to the server implementation.
