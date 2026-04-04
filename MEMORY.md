# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to all interfaces on port 8080 and serves static files from the current directory.

## Architecture

### Current Implementation
The server is implemented in `chirpy.go` with two main components:

1. **makeHandler()** function: Creates and returns the HTTP handler
   - Creates an http.ServeMux (request multiplexer/router)
   - Registers handlers in this order:
     - `/assets/` path: Serves files from the `assets/` directory using `http.StripPrefix` and `http.FileServer`
     - `/` root path: Serves files from the current directory (`.`) using `http.FileServer`
   - Returns the configured handler (testable and reusable)

2. **main()** function: Sets up and starts the server
   - Creates an http.Server struct configured with:
     - `Handler`: Set to the result of `makeHandler()`
     - `Addr`: Set to ":8080" to bind on all interfaces on port 8080
   - Calls `ListenAndServe()` to start the HTTP server

### FileServer Behavior
The server uses multiple `http.FileServer` handlers for different paths:

**Assets Handler** (`/assets/`):
- Serves files from the `assets/` directory
- Uses `http.StripPrefix` to remove `/assets/` prefix from request path
- Example: Request to `/assets/logo.png` serves `assets/logo.png`
- Returns appropriate Content-Type for served files (e.g., `image/png`)

**Root Handler** (`/`):
- Serves files from the current directory (`.`)
- Automatically serves `index.html` when accessing the root path `/`
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

- **TestServeIndexHTML**: Tests that the handler serves `index.html` correctly
  - Verifies the server returns a 200 status code
  - Confirms the response contains "Welcome to Chirpy"
  - Uses the actual `makeHandler()` function from the production code

- **TestServeLogo**: Tests that the handler serves assets correctly
  - Verifies the server returns a 200 status code for `/assets/logo.png`
  - Confirms the Content-Type header indicates an image
  - Verifies the response body contains image file content

Run tests with:
```bash
go test -v
```

### Manual Testing
- **Root Path (`/`)**: Automatically serves `index.html` with a 200 status code
- **Assets Path (`/assets/`)**: Serves files from the `assets/` directory
- **Logo**: Access at `http://localhost:8080/assets/logo.png`

Example:
```bash
curl http://localhost:8080/
curl http://localhost:8080/assets/logo.png
```

## Key Design Decisions
- **Testable Handler**: The `makeHandler()` function is extracted to allow unit testing of the handler logic without starting a full server
- **Explicit Path Routing**: Specific paths (`/assets/`) are registered before catch-all paths (`/`) to ensure correct routing priority
- **Asset Directory Structure**: Assets are organized in a dedicated `assets/` directory, separated from root-level files
- **http.StripPrefix**: Uses `http.StripPrefix` to cleanly map URL paths to filesystem directories (e.g., `/assets/logo.png` → `assets/logo.png`)
- **All Interfaces**: Using `:8080` instead of `localhost:8080` to bind on all interfaces, allowing testing from different machines
- **FileServer Handler**: Uses Go's standard `http.FileServer` to serve static files without custom route logic
- **Standard Library Only**: Uses only Go's `net/http` package (no external dependencies)

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
