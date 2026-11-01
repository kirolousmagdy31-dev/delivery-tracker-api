package common

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
)

func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func OpenDB() *sql.DB {
	dsn := Env("DB_DSN", "root:root@tcp(localhost:3306)/package_tracking?parseTime=true&charset=utf8mb4")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err = db.Ping(); err == nil {
			db.SetMaxOpenConns(20)
			return db
		}
		log.Println("waiting for database:", err)
		time.Sleep(2 * time.Second)
	}
	log.Fatal("database unreachable: ", err)
	return nil
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func PathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func ParseTime(s string) (time.Time, error) {
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid time format")
}

type Claims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type ctxKey struct{}

func secret() []byte { return []byte(Env("JWT_SECRET", "change-me-in-production")) }

func NewToken(userID int64, role string) (string, error) {
	c := Claims{UserID: userID, Role: role, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret())
}

func Protect(h http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			Error(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims := &Claims{}
		tok, err := jwt.ParseWithClaims(strings.TrimPrefix(hdr, "Bearer "), claims,
			func(*jwt.Token) (any, error) { return secret(), nil },
			jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !tok.Valid {
			Error(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if len(roles) > 0 {
			ok := false
			for _, role := range roles {
				if claims.Role == role {
					ok = true
				}
			}
			if !ok {
				Error(w, http.StatusForbidden, "forbidden for role "+claims.Role)
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, claims)))
	}
}

func CurrentUser(r *http.Request) *Claims { return r.Context().Value(ctxKey{}).(*Claims) }

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", Env("CORS_ORIGIN", "*"))
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Serve(name, port string, mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, 200, map[string]string{"service": name, "status": "ok"})
	})
	srv := &http.Server{Addr: ":" + port, Handler: cors(mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second}
	log.Printf("%s listening on :%s", name, port)
	log.Fatal(srv.ListenAndServe())
}
