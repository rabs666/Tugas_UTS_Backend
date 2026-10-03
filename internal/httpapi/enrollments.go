package httpapi

import (
	"database/sql"
	"errors"
	"regexp"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
)

var academicYearPattern = regexp.MustCompile(`^[0-9]{4}/[0-9]{4}-(Ganjil|Genap)$`)

func (api *API) createEnrollment(c *fiber.Ctx) error {
	if !api.requireStudent(c) {
		return fail(c, fiber.StatusForbidden, "Akses hanya untuk mahasiswa")
	}
	var input struct {
		CourseID      int64  `json:"course_id" validate:"required,gt=0"`
		TahunAkademik string `json:"tahun_akademik" validate:"required"`
	}
	valid, err := validate(api, c, &input)
	if err != nil {
		return err
	}
	if !valid {
		return nil
	}
	if !academicYearPattern.MatchString(input.TahunAkademik) {
		return invalid(c, map[string][]string{"tahun_akademik": {"Format harus YYYY/YYYY-Ganjil atau YYYY/YYYY-Genap"}})
	}
	user := c.Locals("user").(*claims)
	tx, err := api.db.BeginTx(c.UserContext(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var studentID int64
	var ipk float64
	err = tx.QueryRowContext(c.UserContext(), `
		SELECT id, ipk_terakhir FROM students
		WHERE user_id = $1 AND deleted_at IS NULL FOR UPDATE`, user.UserID).
		Scan(&studentID, &ipk)
	if err == sql.ErrNoRows {
		return fail(c, fiber.StatusForbidden, "Akun mahasiswa tidak aktif")
	}
	if err != nil {
		return err
	}

	var courseID int64
	var code, name string
	var sks, quota int
	err = tx.QueryRowContext(c.UserContext(), `
		SELECT id, kode_mk, nama_mk, sks, kuota FROM courses
		WHERE id = $1 FOR UPDATE`, input.CourseID).
		Scan(&courseID, &code, &name, &sks, &quota)
	if err == sql.ErrNoRows {
		return fail(c, fiber.StatusUnprocessableEntity, "Mata kuliah tidak ditemukan")
	}
	if err != nil {
		return err
	}
	var duplicate bool
	if err := tx.QueryRowContext(c.UserContext(), `
		SELECT EXISTS (SELECT 1 FROM enrollments
		WHERE student_id = $1 AND course_id = $2 AND tahun_akademik = $3)`,
		studentID, courseID, input.TahunAkademik).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return fail(c, fiber.StatusConflict, "Mata kuliah sudah diambil pada tahun akademik ini")
	}
	var filled int
	if err := tx.QueryRowContext(c.UserContext(),
		"SELECT COUNT(*) FROM enrollments WHERE course_id = $1", courseID).Scan(&filled); err != nil {
		return err
	}
	if filled >= quota {
		return invalid(c, map[string][]string{"course_id": {"Kuota mata kuliah sudah penuh"}})
	}
	var totalSKS int
	if err := tx.QueryRowContext(c.UserContext(), `
		SELECT COALESCE(SUM(c.sks), 0) FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		WHERE e.student_id = $1 AND e.tahun_akademik = $2`,
		studentID, input.TahunAkademik).Scan(&totalSKS); err != nil {
		return err
	}
	limit := creditLimit(ipk)
	remaining := limit - totalSKS
	if totalSKS+sks > limit {
		return invalid(c, map[string][]string{
			"course_id": {"Total SKS melebihi batas; sisa SKS saat ini " + itoa(remaining)},
		})
	}
	var enrollmentID int64
	err = tx.QueryRowContext(c.UserContext(), `
		INSERT INTO enrollments (student_id, course_id, tahun_akademik)
		VALUES ($1, $2, $3) RETURNING id`,
		studentID, courseID, input.TahunAkademik).Scan(&enrollmentID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fail(c, fiber.StatusConflict, "Mata kuliah sudah diambil pada tahun akademik ini")
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(response{
		Success: true, Message: "Mata kuliah berhasil ditambahkan ke KRS",
		Data: map[string]interface{}{
			"id": enrollmentID, "course_id": courseID, "kode_mk": code,
			"nama_mk": name, "sks": sks, "tahun_akademik": input.TahunAkademik,
		},
	})
}

func (api *API) deleteEnrollment(c *fiber.Ctx) error {
	if !api.requireStudent(c) {
		return fail(c, fiber.StatusForbidden, "Akses hanya untuk mahasiswa")
	}
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fail(c, fiber.StatusNotFound, "KRS tidak ditemukan")
	}
	user := c.Locals("user").(*claims)
	var ownerID int64
	err = api.db.QueryRowContext(c.UserContext(), `
		SELECT s.user_id FROM enrollments e
		JOIN students s ON s.id = e.student_id
		WHERE e.id = $1 AND s.deleted_at IS NULL`, id).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return fail(c, fiber.StatusNotFound, "KRS tidak ditemukan")
	}
	if err != nil {
		return err
	}
	if ownerID != user.UserID {
		return fail(c, fiber.StatusForbidden, "Mahasiswa hanya dapat membatalkan KRS miliknya sendiri")
	}
	result, err := api.db.ExecContext(c.UserContext(), "DELETE FROM enrollments WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fail(c, fiber.StatusNotFound, "KRS tidak ditemukan")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
