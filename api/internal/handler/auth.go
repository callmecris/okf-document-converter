package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"okf/api/internal/middleware"
	"okf/api/internal/repository"
	"okf/pkg/domain"
)

const tokenTTL = 24 * time.Hour

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Auth struct {
	repo      *repository.Postgres
	jwtSecret string
	log       *slog.Logger
}

func NewAuth(repo *repository.Postgres, jwtSecret string, log *slog.Logger) *Auth {
	return &Auth{repo: repo, jwtSecret: jwtSecret, log: log}
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

// Register crea un usuario y devuelve un token JWT.
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo de solicitud inválido")
		return
	}
	if !emailRe.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "correo inválido")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "la contraseña debe tener al menos 8 caracteres")
		return
	}

	if _, err := a.repo.GetUserByEmail(r.Context(), req.Email); err == nil {
		writeError(w, http.StatusConflict, "ya existe una cuenta con ese correo")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, a.log, err)
		return
	}

	user, err := a.repo.CreateUser(r.Context(), repository.CreateUserParams{
		ID:           uuid.NewString(),
		Email:        req.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		internalError(w, a.log, err)
		return
	}
	a.respond(w, user)
}

// Login valida credenciales y devuelve un token JWT.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo de solicitud inválido")
		return
	}

	user, err := a.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user.ID == "" {
		writeError(w, http.StatusUnauthorized, "correo o contraseña incorrectos")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "correo o contraseña incorrectos")
		return
	}
	a.respond(w, user)
}

func (a *Auth) respond(w http.ResponseWriter, user domain.User) {
	token, err := middleware.GenerateToken(a.jwtSecret, user.ID, tokenTTL)
	if err != nil {
		internalError(w, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, User: user})
}