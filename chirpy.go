package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ach1000/chirpy/internal/auth"
	"github.com/ach1000/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	jwtSecret      string
}

func makeHandler() http.Handler {
	return makeHandlerWithConfig(&apiConfig{platform: "dev"})
}

func makeHandlerWithConfig(apiCfg *apiConfig) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/users", apiCfg.handlerUsersCreate)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirpsCreate)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpsGet)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerChirpsGetByID)

	return mux
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: error loading .env file: %v", err)
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL must be set")
	}
	platform := os.Getenv("PLATFORM")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	dbQueries := database.New(db)
	apiCfg := &apiConfig{dbQueries: dbQueries, platform: platform, jwtSecret: jwtSecret}

	server := &http.Server{
		Addr:    ":8080",
		Handler: makeHandlerWithConfig(apiCfg),
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, http.StatusForbidden, "Reset is only allowed in dev environment")
		return
	}

	if cfg.dbQueries != nil {
		if err := cfg.dbQueries.DeleteUsers(r.Context()); err != nil {
			log.Printf("Error deleting users: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Couldn't reset users")
			return
		}
	}

	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

type accessTokenResponse struct {
	Token string `json:"token"`
}

type userResponse struct {
	ID           string `json:"id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Email        string `json:"email"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func newUserResponse(user database.User) userResponse {
	return userResponse{
		ID:        user.ID.String(),
		CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.UTC().Format(time.RFC3339),
		Email:     user.Email,
	}
}

func (cfg *apiConfig) handlerUsersCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if params.Email == "" || params.Password == "" {
		respondWithError(w, http.StatusBadRequest, "Email and password are required")
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
		return
	}

	respondWithJSON(w, http.StatusCreated, newUserResponse(user))
}

const accessTokenExpiry = time.Hour
const refreshTokenExpiry = 60 * 24 * time.Hour

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	passwordMatches, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil || !passwordMatches {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	token, err := auth.MakeJWT(user.ID, cfg.jwtSecret, accessTokenExpiry)
	if err != nil {
		log.Printf("Error creating JWT: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create token")
		return
	}

	refreshToken, err := cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     auth.MakeRefreshToken(),
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(refreshTokenExpiry),
	})
	if err != nil {
		log.Printf("Error creating refresh token: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create token")
		return
	}

	response := newUserResponse(user)
	response.Token = token
	response.RefreshToken = refreshToken.Token
	respondWithJSON(w, http.StatusOK, response)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	token, err := auth.MakeJWT(user.ID, cfg.jwtSecret, accessTokenExpiry)
	if err != nil {
		log.Printf("Error creating JWT: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create token")
		return
	}

	respondWithJSON(w, http.StatusOK, accessTokenResponse{Token: token})
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := cfg.dbQueries.RevokeRefreshToken(r.Context(), refreshToken); err != nil {
		log.Printf("Error revoking refresh token: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

const maxChirpLength = 140

var chirpTokenPattern = regexp.MustCompile(`\S+|\s+`)

func cleanChirp(body string) string {
	profaneWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}

	return chirpTokenPattern.ReplaceAllStringFunc(body, func(token string) string {
		if _, isProfane := profaneWords[strings.ToLower(token)]; isProfane {
			return "****"
		}

		return token
	})
}

type chirpResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body"`
	UserID    string `json:"user_id"`
}

func newChirpResponse(chirp database.Chirp) chirpResponse {
	return chirpResponse{
		ID:        chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: chirp.UpdatedAt.UTC().Format(time.RFC3339),
		Body:      chirp.Body,
		UserID:    chirp.UserID.String(),
	}
}

func (cfg *apiConfig) handlerChirpsCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	if len(params.Body) > maxChirpLength {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleanChirp(params.Body),
		UserID: userID,
	})
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp")
		return
	}

	respondWithJSON(w, http.StatusCreated, newChirpResponse(chirp))
}

func (cfg *apiConfig) handlerChirpsGet(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	chirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error getting chirps: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't get chirps")
		return
	}

	response := make([]chirpResponse, len(chirps))
	for i, c := range chirps {
		response[i] = newChirpResponse(c)
	}

	respondWithJSON(w, http.StatusOK, response)
}

func (cfg *apiConfig) handlerChirpsGetByID(w http.ResponseWriter, r *http.Request) {
	if cfg.dbQueries == nil {
		respondWithError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}

	chirp, err := cfg.dbQueries.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Chirp not found")
			return
		}

		log.Printf("Error getting chirp by ID: %s", err)
		respondWithError(w, http.StatusInternalServerError, "Couldn't get chirp")
		return
	}

	respondWithJSON(w, http.StatusOK, newChirpResponse(chirp))
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, struct {
		Error string `json:"error"`
	}{Error: msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}
