// user-service: Feature #1 (register) and #2 (login), plus courier management for admins.
package user

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"package-tracking/internal/common"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type app struct{ db *sql.DB }

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	Role  string `json:"role"`
}

type userInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (in *userInput) normalizeAndValidate() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Phone = strings.TrimSpace(in.Phone)
	switch {
	case len(in.Name) < 2 || len(in.Name) > 100:
		return "name must be 2-100 characters"
	case !emailRe.MatchString(in.Email):
		return "invalid email"
	case !common.PhoneRe.MatchString(in.Phone):
		return "phone must be 8-15 digits (optional leading +)"
	case len(in.Password) < 6:
		return "password must be at least 6 characters"
	}
	return ""
}

func (a *app) createUser(in userInput, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := a.db.Exec(`INSERT INTO users (name,email,phone,password_hash,role) VALUES (?,?,?,?,?)`,
		in.Name, in.Email, in.Phone, string(hash), role)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Name: in.Name, Email: in.Email, Phone: in.Phone, Role: role}, nil
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (a *app) signup(role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in userInput
		if !common.Decode(w, r, &in) {
			return
		}
		if msg := in.normalizeAndValidate(); msg != "" {
			common.Error(w, http.StatusBadRequest, msg)
			return
		}
		u, err := a.createUser(in, role)
		if isDuplicate(err) {
			common.Error(w, http.StatusConflict, "email already registered")
			return
		}
		if err != nil {
			log.Println(err)
			common.Error(w, http.StatusInternalServerError, "could not create account")
			return
		}
		common.JSON(w, http.StatusCreated, u)
	}
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" || in.Password == "" {
		common.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}
	var u User
	var hash string
	err := a.db.QueryRow(`SELECT id,name,email,phone,role,password_hash FROM users WHERE email=?`, in.Email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Role, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		common.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	token, err := common.NewToken(u.ID, u.Role)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "could not create token")
		return
	}
	common.JSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	var u User
	err := a.db.QueryRow(`SELECT id,name,email,phone,role FROM users WHERE id=?`, common.CurrentUser(r).UserID).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Role)
	if err != nil {
		common.Error(w, http.StatusNotFound, "user not found")
		return
	}
	common.JSON(w, http.StatusOK, u)
}

func (a *app) listCouriers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id,name,email,phone,role FROM users WHERE role='courier' ORDER BY name`)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Role); err == nil {
			out = append(out, u)
		}
	}
	common.JSON(w, http.StatusOK, out)
}

// seed creates a default admin and courier so the system is usable immediately.
func (a *app) seed() {
	for _, s := range []struct{ role, name, email, pass, phone string }{
		{"admin", "Admin", common.Env("ADMIN_EMAIL", "admin@example.com"), common.Env("ADMIN_PASSWORD", "Admin@123"), "01000000000"},
		{"courier", "Demo Courier", common.Env("COURIER_EMAIL", "courier@example.com"), common.Env("COURIER_PASSWORD", "Courier@123"), "01111111111"},
		{"courier", "Demo Courier2", common.Env("COURIER_EMAIL", "courier2@example.com"), common.Env("COURIER_PASSWORD", "Courier@123"), "01111111111"},
	} {
		var n int
		_ = a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email=?`, s.email).Scan(&n)
		if n == 0 {
			if _, err := a.createUser(userInput{Name: s.name, Email: s.email, Phone: s.phone, Password: s.pass}, s.role); err != nil {
				log.Println("seed", s.role, err)
			}
		}
	}
}

func Register(mux *http.ServeMux, db *sql.DB) {
	a := &app{db: db}
	a.seed()

	mux.HandleFunc("POST /api/auth/register", a.signup("user"))                              // Feature #1
	mux.HandleFunc("POST /api/auth/login", a.login)                                          // Feature #2
	mux.HandleFunc("GET /api/users/me", common.Protect(a.me))                                // current profile
	mux.HandleFunc("GET /api/users/couriers", common.Protect(a.listCouriers, "admin"))       // for reassign dropdown
	mux.HandleFunc("POST /api/users/couriers", common.Protect(a.signup("courier"), "admin")) // admin creates couriers
}
