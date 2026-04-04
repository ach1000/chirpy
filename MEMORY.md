# Chirpy Server - Project Documentation

## Overview
This is a simple Go HTTP fileserver that binds to all interfaces on port 8080 and serves static files from the current directory.

## Architecture

### Current Implementation
The server is implemented in `chirpy.go` with two main components:

1. **makeHandler()** function: Creates and returns the HTTP handler
   - Creates an http.ServeMux (request multiplexer/router)
   - Registers a FileServer Handler using `http.FileServer` with `http.Dir(".")` to serve files from the current directory at root path "/"
   - Returns the configured handler (testable and reusable)

2. **main()** function: Sets up and starts the server
   - Creates an http.Server struct configured with:
     - `Handler`: Set to the result of `makeHandler()`
     - `Addr`: Set to ":8080" to bind on all interfaces on port 8080
   - Calls `ListenAndServe()` to start the HTTP server

### FileServer Behavior
The `http.FileServer` handler:
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

Run tests with:
```bash
go test -v
```

### Manual Testing
- **Root Path (`/`)**: Automatically serves `index.html` with a 200 status code
- **Direct File Access**: `/index.html` redirects to `/`

Example:
```bash
curl http://localhost:8080/
```

Expected response:
```html
<html>
  <body>
    <h1>Welcome to Chirpy</h1>
  </body>
</html>
```

## Key Design Decisions
- **Testable Handler**: The `makeHandler()` function is extracted to allow unit testing of the handler logic without starting a full server
- **All Interfaces**: Using `:8080` instead of `localhost:8080` to bind on all interfaces, allowing testing from different machines
- **FileServer Handler**: Uses Go's standard `http.FileServer` to serve static files without custom route logic
- **Current Directory**: Uses `http.Dir(".")` to serve files from the project root directory
- **Standard Library Only**: Uses only Go's `net/http` package (no external dependencies)

## Project Structure
```
chirpy/
├── chirpy.go       # Server implementation with makeHandler() function
├── chirpy_test.go  # Unit tests for the server
├── index.html      # Static HTML file served at root
├── go.mod          # Go module definition
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
