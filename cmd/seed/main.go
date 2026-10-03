package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rabs666/Tugas_UTS_Backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type course struct {
	code, name string
	credits    int
	semester   int
	capacity   int
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := seed(ctx, db); err != nil {
		log.Fatal(err)
	}
	log.Println("Seeder selesai: 1 admin, 20 mahasiswa, dan 10 mata kuliah.")
}

func seed(ctx context.Context, db *sql.DB) error {
	adminEmail := envOr("ADMIN_EMAIL", "admin@siakad.local")
	adminPassword := envOr("ADMIN_PASSWORD", "Admin123!")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	adminHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (email, password, role) VALUES ($1, $2, 'admin')
		ON CONFLICT (email) DO UPDATE SET password = EXCLUDED.password, role = EXCLUDED.role`,
		adminEmail, string(adminHash)); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	for index := 1; index <= 20; index++ {
		nim := fmt.Sprintf("2026%08d", index)
		email := fmt.Sprintf("mahasiswa%02d@siakad.local", index)
		hash, err := bcrypt.GenerateFromPassword([]byte(nim), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		var userID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO users (email, password, role) VALUES ($1, $2, 'mahasiswa')
			ON CONFLICT (email) DO UPDATE SET role = EXCLUDED.role
			RETURNING id`, email, string(hash)).Scan(&userID)
		if err != nil {
			return fmt.Errorf("seed user %s: %w", email, err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO students (user_id, nim, nama, prodi, angkatan, ipk_terakhir)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (nim) DO NOTHING`,
			userID, nim, fmt.Sprintf("Mahasiswa %02d", index), programs[(index-1)%len(programs)],
			2026, gradePoints[(index-1)%len(gradePoints)])
		if err != nil {
			return fmt.Errorf("seed student %s: %w", nim, err)
		}
	}
	for _, item := range courses {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO courses (kode_mk, nama_mk, sks, semester, kuota)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (kode_mk) DO NOTHING`,
			item.code, item.name, item.credits, item.semester, item.capacity); err != nil {
			return fmt.Errorf("seed course %s: %w", item.code, err)
		}
	}
	return tx.Commit()
}

var programs = []string{"Sistem Informasi", "Informatika", "Teknologi Informasi", "Manajemen Informatika"}
var gradePoints = []float64{3.45, 2.85, 2.30, 3.10, 3.70}
var courses = []course{
	{"IF101", "Algoritma dan Pemrograman", 3, 1, 30},
	{"IF102", "Basis Data", 3, 2, 30},
	{"IF103", "Pemrograman Web", 3, 3, 30},
	{"IF104", "Jaringan Komputer", 3, 4, 30},
	{"IF105", "Rekayasa Perangkat Lunak", 3, 5, 30},
	{"IF106", "Sistem Operasi", 3, 3, 30},
	{"IF107", "Analisis dan Desain Sistem", 2, 4, 30},
	{"IF108", "Keamanan Informasi", 2, 6, 30},
	{"IF109", "Kecerdasan Buatan", 3, 7, 30},
	{"IF110", "Manajemen Proyek TI", 2, 8, 30},
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
