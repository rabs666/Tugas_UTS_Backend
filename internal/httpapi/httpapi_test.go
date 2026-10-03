package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestCreditLimit(t *testing.T) {
	tests := []struct {
		ipk  float64
		want int
	}{
		{4, 24}, {3, 24}, {2.99, 21}, {2.5, 21}, {2.49, 18}, {0, 18},
	}
	for _, test := range tests {
		if got := creditLimit(test.ipk); got != test.want {
			t.Errorf("creditLimit(%v) = %d, want %d", test.ipk, got, test.want)
		}
	}
}

func TestAcademicYearPattern(t *testing.T) {
	for _, year := range []string{"2026/2027-Ganjil", "2026/2027-Genap"} {
		if !academicYearPattern.MatchString(year) {
			t.Errorf("expected %q to be valid", year)
		}
	}
	for _, year := range []string{"2026-2027-Ganjil", "2026/2027-genap", "2026/2027"} {
		if academicYearPattern.MatchString(year) {
			t.Errorf("expected %q to be invalid", year)
		}
	}
}

func TestLoginLimiterAllowsFiveFailedAttempts(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	for attempt := 0; attempt < 5; attempt++ {
		if limiter.tooMany("127.0.0.1", now) {
			t.Fatalf("attempt %d was rejected before five failures", attempt+1)
		}
		limiter.failed("127.0.0.1", now)
	}
	if !limiter.tooMany("127.0.0.1", now) {
		t.Fatal("sixth attempt should be rate limited")
	}
	limiter.succeeded("127.0.0.1")
	if limiter.tooMany("127.0.0.1", now) {
		t.Fatal("successful login should clear the failure count")
	}
}

func TestLoginLimiterExpiresAfterOneMinute(t *testing.T) {
	var limiter loginLimiter
	now := time.Now()
	for attempt := 0; attempt < 5; attempt++ {
		limiter.failed("127.0.0.1", now)
	}
	if limiter.tooMany("127.0.0.1", now.Add(time.Minute)) {
		t.Fatal("expired attempts should no longer be rate limited")
	}
}

func TestLoginValidationUsesJSONFieldNames(t *testing.T) {
	app := New(nil, "test-secret-with-more-than-32-characters", time.Hour).Router()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewBufferString(`{"email":"not-an-email","password":"short"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusUnprocessableEntity)
	}
	var body struct {
		Errors map[string][]string `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Errors["email"]) == 0 || len(body.Errors["password"]) == 0 {
		t.Fatalf("validation errors must use JSON field names, got %#v", body.Errors)
	}
}

func TestProtectedEndpointRequiresToken(t *testing.T) {
	app := New(nil, "test-secret-with-more-than-32-characters", time.Hour).Router()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestRequiredRoutesAreRegistered(t *testing.T) {
	app := New(nil, "test-secret-with-more-than-32-characters", time.Hour).Router()
	expected := map[string]bool{
		"POST /api/v1/auth/login":        false,
		"GET /api/v1/auth/me":            false,
		"GET /api/v1/students":           false,
		"POST /api/v1/students":          false,
		"GET /api/v1/students/:id":       false,
		"PUT /api/v1/students/:id":       false,
		"DELETE /api/v1/students/:id":    false,
		"GET /api/v1/courses":            false,
		"POST /api/v1/enrollments":       false,
		"DELETE /api/v1/enrollments/:id": false,
	}
	for _, route := range app.GetRoutes() {
		key := route.Method + " " + route.Path
		if _, ok := expected[key]; ok {
			expected[key] = true
		}
	}
	for route, registered := range expected {
		if !registered {
			t.Errorf("required route %q is not registered", route)
		}
	}
}

func TestAdminOnlyStudentListRejectsStudentRole(t *testing.T) {
	api := New(nil, "test-secret-with-more-than-32-characters", time.Hour)
	app := fiber.New()
	app.Get("/students", func(c *fiber.Ctx) error {
		c.Locals("user", &claims{Role: "mahasiswa"})
		return api.listStudents(c)
	})
	req, err := http.NewRequest(http.MethodGet, "/students", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusForbidden)
	}
}

func TestStudentEnrollmentRejectsAdminRole(t *testing.T) {
	api := New(nil, "test-secret-with-more-than-32-characters", time.Hour)
	app := fiber.New()
	app.Post("/enrollments", func(c *fiber.Ctx) error {
		c.Locals("user", &claims{Role: "admin"})
		return api.createEnrollment(c)
	})
	req, err := http.NewRequest(http.MethodPost, "/enrollments", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusForbidden)
	}
}
