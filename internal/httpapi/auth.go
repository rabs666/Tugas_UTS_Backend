package httpapi

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type loginAttempt struct {
	count   int
	expires time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func (api *API) login(c *fiber.Ctx) error {
	var input struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,min=8"`
	}
	if err := validate(api, c, &input); err != nil {
		return err
	}
	ip := c.IP()
	if api.loginLock.tooMany(ip, time.Now()) {
		return fail(c, fiber.StatusTooManyRequests, "Terlalu banyak percobaan login; coba lagi dalam satu menit")
	}

	var userID int64
	var email, role, hash string
	err := api.db.QueryRowContext(c.UserContext(), `
		SELECT u.id, u.email, u.role, u.password
		FROM users u
		LEFT JOIN students s ON s.user_id = u.id
		WHERE u.email = $1 AND (u.role <> 'mahasiswa' OR s.deleted_at IS NULL)`,
		input.Email).Scan(&userID, &email, &role, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		api.loginLock.failed(ip, time.Now())
		return fail(c, fiber.StatusUnauthorized, "Email atau password salah")
	}
	api.loginLock.succeeded(ip)

	accessToken, err := api.issueToken(userID, email, role)
	if err != nil {
		return err
	}
	return c.JSON(response{
		Success: true, Message: "Login berhasil",
		Data: map[string]interface{}{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   int(api.tokenTTL.Seconds()),
			"user":         map[string]interface{}{"id": userID, "email": email, "role": role},
		},
	})
}

func (l *loginLimiter) tooMany(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.attempts[key]
	return ok && now.Before(attempt.expires) && attempt.count >= 5
}

func (l *loginLimiter) failed(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.attempts == nil {
		l.attempts = make(map[string]loginAttempt)
	}
	attempt := l.attempts[key]
	if !now.Before(attempt.expires) {
		attempt = loginAttempt{expires: now.Add(time.Minute)}
	}
	attempt.count++
	l.attempts[key] = attempt
	for ip, existing := range l.attempts {
		if !now.Before(existing.expires) {
			delete(l.attempts, ip)
		}
	}
}

func (l *loginLimiter) succeeded(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (api *API) me(c *fiber.Ctx) error {
	user := c.Locals("user").(*claims)
	result := map[string]interface{}{"id": user.UserID, "email": user.Email, "role": user.Role}
	if user.Role == "mahasiswa" {
		var nim, nama, prodi string
		var angkatan int
		err := api.db.QueryRowContext(c.UserContext(), `
			SELECT nim, nama, prodi, angkatan FROM students
			WHERE user_id = $1 AND deleted_at IS NULL`, user.UserID).
			Scan(&nim, &nama, &prodi, &angkatan)
		if err != nil {
			return err
		}
		result["student"] = map[string]interface{}{
			"nim": nim, "nama": nama, "prodi": prodi, "angkatan": angkatan,
		}
	}
	return c.JSON(response{Success: true, Message: "Profil pengguna berhasil diambil", Data: result})
}
