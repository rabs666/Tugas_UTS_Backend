package httpapi

import (
	"database/sql"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type API struct {
	db        *sql.DB
	secret    []byte
	tokenTTL  time.Duration
	validator *validator.Validate
	loginLock loginLimiter
}

type claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
	Errors  interface{} `json:"errors,omitempty"`
}

func New(db *sql.DB, secret string, ttl time.Duration) *API {
	return &API{db: db, secret: []byte(secret), tokenTTL: ttl, validator: validator.New()}
}

func (api *API) Router() *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			if code >= fiber.StatusInternalServerError {
				log.Printf("request %s %s failed: %v", c.Method(), c.Path(), err)
			}
			return c.Status(code).JSON(response{Success: false, Message: "Terjadi kesalahan pada server"})
		},
	})
	app.Use(func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		return c.Next()
	})

	v1 := app.Group("/api/v1")
	v1.Post("/auth/login", api.login)
	protected := v1.Group("", api.authenticate)
	protected.Get("/auth/me", api.me)
	protected.Get("/students", api.listStudents)
	protected.Post("/students", api.createStudent)
	protected.Get("/students/:id", api.getStudent)
	protected.Put("/students/:id", api.updateStudent)
	protected.Delete("/students/:id", api.deleteStudent)
	protected.Get("/courses", api.listCourses)
	protected.Post("/enrollments", api.createEnrollment)
	protected.Delete("/enrollments/:id", api.deleteEnrollment)
	return app
}

func (api *API) authenticate(c *fiber.Ctx) error {
	header := c.Get(fiber.HeaderAuthorization)
	if len(header) < 8 || header[:7] != "Bearer " {
		return fail(c, fiber.StatusUnauthorized, "Token tidak valid atau tidak tersedia")
	}
	parsed, err := jwt.ParseWithClaims(header[7:], &claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fiber.ErrUnauthorized
		}
		return api.secret, nil
	})
	if err != nil || !parsed.Valid {
		return fail(c, fiber.StatusUnauthorized, "Token tidak valid atau kedaluwarsa")
	}
	userClaims, ok := parsed.Claims.(*claims)
	if !ok || userClaims.UserID < 1 || (userClaims.Role != "admin" && userClaims.Role != "mahasiswa") {
		return fail(c, fiber.StatusUnauthorized, "Token tidak valid")
	}
	if userClaims.Role == "mahasiswa" {
		var active bool
		err = api.db.QueryRowContext(c.UserContext(), `
			SELECT EXISTS (
				SELECT 1 FROM students WHERE user_id = $1 AND deleted_at IS NULL
			)`, userClaims.UserID).Scan(&active)
		if err != nil {
			return err
		}
		if !active {
			return fail(c, fiber.StatusUnauthorized, "Akun mahasiswa tidak aktif")
		}
	}
	c.Locals("user", userClaims)
	return c.Next()
}

func (api *API) requireAdmin(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(*claims)
	if user == nil || user.Role != "admin" {
		return fail(c, fiber.StatusForbidden, "Akses hanya untuk admin")
	}
	return c.Next()
}

func (api *API) requireStudent(c *fiber.Ctx) error {
	user, _ := c.Locals("user").(*claims)
	if user == nil || user.Role != "mahasiswa" {
		return fail(c, fiber.StatusForbidden, "Akses hanya untuk mahasiswa")
	}
	return c.Next()
}

func (api *API) issueToken(userID int64, email, role string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(api.tokenTTL)),
		},
	})
	return token.SignedString(api.secret)
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, fiber.ErrNotFound
	}
	return id, nil
}

func fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(response{Success: false, Message: message})
}

func invalid(c *fiber.Ctx, errs map[string][]string) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(response{
		Success: false, Message: "Validasi gagal", Errors: errs,
	})
}

func validate(api *API, c *fiber.Ctx, input interface{}) error {
	if err := c.BodyParser(input); err != nil {
		return invalid(c, map[string][]string{"body": {"Format JSON tidak valid"}})
	}
	if err := api.validator.Struct(input); err != nil {
		validationErr, ok := err.(validator.ValidationErrors)
		if !ok {
			return err
		}
		errs := make(map[string][]string)
		for _, fieldErr := range validationErr {
			name := jsonFieldName(input, fieldErr.StructField())
			switch fieldErr.Tag() {
			case "required":
				errs[name] = append(errs[name], "Wajib diisi")
			case "email":
				errs[name] = append(errs[name], "Format email tidak valid")
			case "len":
				errs[name] = append(errs[name], "Panjang nilai tidak sesuai")
			case "numeric":
				errs[name] = append(errs[name], "Harus berupa angka")
			case "min":
				errs[name] = append(errs[name], "Nilai di bawah batas minimum")
			case "max":
				errs[name] = append(errs[name], "Nilai melebihi batas maksimum")
			case "gt":
				errs[name] = append(errs[name], "Nilai harus lebih besar dari batas minimum")
			case "gte":
				errs[name] = append(errs[name], "Nilai di bawah batas minimum")
			case "lte":
				errs[name] = append(errs[name], "Nilai melebihi batas maksimum")
			default:
				errs[name] = append(errs[name], "Nilai tidak valid")
			}
		}
		return invalid(c, errs)
	}
	return nil
}

func jsonFieldName(input interface{}, structField string) string {
	typeOf := reflect.TypeOf(input)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	field, ok := typeOf.FieldByName(structField)
	if !ok {
		return structField
	}
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "" || name == "-" {
		return structField
	}
	return name
}

func passwordHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
