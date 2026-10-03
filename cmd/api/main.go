package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/rabs666/Tugas_UTS_Backend/internal/database"
	"github.com/rabs666/Tugas_UTS_Backend/internal/httpapi"
)

func main() {
	connectionString := os.Getenv("DATABASE_URL")
	if connectionString == "" {
		log.Fatal("DATABASE_URL wajib diatur")
	}
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 || strings.Contains(strings.ToLower(secret), "replace-this") {
		log.Fatal("JWT_SECRET wajib berisi minimal 32 karakter acak; lihat .env.example")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.Open(ctx, connectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "3000"
	}
	api := httpapi.New(db, secret, envDurationHours("JWT_TTL_HOURS", 24))
	log.Printf("SIAKAD Mini API listening on :%s", port)
	if err := api.Router().Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}

func envDurationHours(name string, fallback int) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return time.Duration(fallback) * time.Hour
	}
	duration, err := time.ParseDuration(value + "h")
	if err != nil || duration <= 0 {
		log.Fatalf("%s harus berupa bilangan jam positif", name)
	}
	return duration
}
