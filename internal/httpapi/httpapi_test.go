package httpapi

import (
	"testing"
	"time"
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
