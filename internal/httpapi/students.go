package httpapi

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type studentInput struct {
	NIM         string  `json:"nim" validate:"required,len=12,numeric"`
	Nama        string  `json:"nama" validate:"required,max=120"`
	Email       string  `json:"email" validate:"required,email"`
	Prodi       string  `json:"prodi" validate:"required,max=120"`
	Angkatan    int     `json:"angkatan" validate:"required,gte=2000,lte=9999"`
	IPKTerakhir float64 `json:"ipk_terakhir" validate:"gte=0,lte=4"`
}

type studentUpdateInput struct {
	Nama        string  `json:"nama" validate:"required,max=120"`
	Prodi       string  `json:"prodi" validate:"required,max=120"`
	Angkatan    int     `json:"angkatan" validate:"required,gte=2000,lte=9999"`
	IPKTerakhir float64 `json:"ipk_terakhir" validate:"gte=0,lte=4"`
}

func (api *API) listStudents(c *fiber.Ctx) error {
	if err := api.requireAdmin(c); err != nil {
		return err
	}
	page, err := queryInt(c, "page", 1, 1, 2147483647)
	if err != nil {
		return fail(c, fiber.StatusUnprocessableEntity, "Parameter page tidak valid")
	}
	perPage, err := queryInt(c, "per_page", 10, 1, 50)
	if err != nil {
		return fail(c, fiber.StatusUnprocessableEntity, "Parameter per_page harus antara 1 dan 50")
	}
	where := []string{"s.deleted_at IS NULL"}
	args := []interface{}{}
	addFilter := func(condition string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if prodi := strings.TrimSpace(c.Query("prodi")); prodi != "" {
		addFilter("s.prodi = $%d", prodi)
	}
	if angkatan := c.Query("angkatan"); angkatan != "" {
		year, err := strconv.Atoi(angkatan)
		if err != nil || year < 2000 || year > 9999 {
			return fail(c, fiber.StatusUnprocessableEntity, "Parameter angkatan tidak valid")
		}
		addFilter("s.angkatan = $%d", year)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(s.nim LIKE $%d OR s.nama ILIKE $%d)", len(args), len(args)))
	}
	order := "s.nama ASC"
	switch c.Query("sort", "nama") {
	case "nama":
		order = "s.nama ASC"
	case "-ipk_terakhir":
		order = "s.ipk_terakhir DESC, s.nama ASC"
	default:
		return fail(c, fiber.StatusUnprocessableEntity, "Parameter sort hanya mendukung nama atau -ipk_terakhir")
	}
	filter := strings.Join(where, " AND ")
	var total int
	if err := api.db.QueryRowContext(c.UserContext(),
		"SELECT COUNT(*) FROM students s WHERE "+filter, args...).Scan(&total); err != nil {
		return err
	}
	lastPage := (total + perPage - 1) / perPage
	if lastPage == 0 {
		lastPage = 1
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := api.db.QueryContext(c.UserContext(), `
		SELECT s.id, s.nim, s.nama, s.prodi, s.angkatan, s.ipk_terakhir
		FROM students s WHERE `+filter+`
		ORDER BY `+order+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	data := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var nim, nama, prodi string
		var angkatan int
		var ipk float64
		if err := rows.Scan(&id, &nim, &nama, &prodi, &angkatan, &ipk); err != nil {
			return err
		}
		data = append(data, map[string]interface{}{
			"id": id, "nim": strings.TrimSpace(nim), "nama": nama, "prodi": prodi,
			"angkatan": angkatan, "ipk_terakhir": ipk,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(response{
		Success: true, Message: "Data mahasiswa berhasil diambil", Data: data,
		Meta: map[string]int{"current_page": page, "per_page": perPage, "total": total, "last_page": lastPage},
	})
}

func (api *API) createStudent(c *fiber.Ctx) error {
	if err := api.requireAdmin(c); err != nil {
		return err
	}
	var input studentInput
	if err := validate(api, c, &input); err != nil {
		return err
	}
	if input.Angkatan != time.Now().Year() {
		return invalid(c, map[string][]string{"angkatan": {"Harus sama dengan tahun berjalan"}})
	}
	if strings.TrimSpace(input.Nama) == "" || strings.TrimSpace(input.Prodi) == "" {
		return invalid(c, map[string][]string{"nama": {"Tidak boleh hanya berisi spasi"}})
	}
	input.Nama, input.Prodi = strings.TrimSpace(input.Nama), strings.TrimSpace(input.Prodi)
	password, err := passwordHash(input.NIM)
	if err != nil {
		return err
	}
	tx, err := api.db.BeginTx(c.UserContext(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID, studentID int64
	err = tx.QueryRowContext(c.UserContext(),
		"INSERT INTO users (email, password, role) VALUES ($1, $2, 'mahasiswa') RETURNING id",
		input.Email, password).Scan(&userID)
	if err != nil {
		return uniqueConflict(c, err, "email", "Email sudah terdaftar")
	}
	err = tx.QueryRowContext(c.UserContext(), `
		INSERT INTO students (user_id, nim, nama, prodi, angkatan, ipk_terakhir)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, input.NIM, input.Nama, input.Prodi, input.Angkatan, input.IPKTerakhir).Scan(&studentID)
	if err != nil {
		return uniqueConflict(c, err, "nim", "NIM sudah terdaftar")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(response{
		Success: true, Message: "Mahasiswa berhasil ditambahkan",
		Data: map[string]interface{}{
			"id": studentID, "nim": input.NIM, "nama": input.Nama,
			"email": input.Email, "prodi": input.Prodi, "angkatan": input.Angkatan,
			"ipk_terakhir": input.IPKTerakhir,
		},
	})
}

func (api *API) getStudent(c *fiber.Ctx) error {
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	user := c.Locals("user").(*claims)
	var ownerUserID int64
	var nim, nama, prodi string
	var angkatan int
	var ipk float64
	err = api.db.QueryRowContext(c.UserContext(), `
		SELECT user_id, nim, nama, prodi, angkatan, ipk_terakhir
		FROM students WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&ownerUserID, &nim, &nama, &prodi, &angkatan, &ipk)
	if err == sql.ErrNoRows {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	if err != nil {
		return err
	}
	if user.Role != "admin" && ownerUserID != user.UserID {
		return fail(c, fiber.StatusForbidden, "Mahasiswa hanya dapat mengakses profilnya sendiri")
	}
	rows, err := api.db.QueryContext(c.UserContext(), `
		SELECT e.id, e.tahun_akademik, c.id, c.kode_mk, c.nama_mk, c.sks
		FROM enrollments e JOIN courses c ON c.id = e.course_id
		WHERE e.student_id = $1 ORDER BY e.created_at DESC`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	courses := make([]map[string]interface{}, 0)
	totalSKS := 0
	for rows.Next() {
		var enrollmentID, courseID int64
		var year, code, title string
		var sks int
		if err := rows.Scan(&enrollmentID, &year, &courseID, &code, &title, &sks); err != nil {
			return err
		}
		courses = append(courses, map[string]interface{}{
			"enrollment_id": enrollmentID, "course_id": courseID,
			"kode_mk": code, "nama_mk": title, "sks": sks, "tahun_akademik": year,
		})
		totalSKS += sks
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(response{
		Success: true, Message: "Detail mahasiswa berhasil diambil",
		Data: map[string]interface{}{
			"id": id, "nim": strings.TrimSpace(nim), "nama": nama, "prodi": prodi,
			"angkatan": angkatan, "ipk_terakhir": ipk, "courses": courses,
			"total_sks": totalSKS, "batas_sks": creditLimit(ipk),
		},
	})
}

func (api *API) updateStudent(c *fiber.Ctx) error {
	if err := api.requireAdmin(c); err != nil {
		return err
	}
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	var input studentUpdateInput
	if err := validate(api, c, &input); err != nil {
		return err
	}
	if strings.TrimSpace(input.Nama) == "" || strings.TrimSpace(input.Prodi) == "" {
		return invalid(c, map[string][]string{"nama": {"Nama dan prodi tidak boleh hanya berisi spasi"}})
	}
	result, err := api.db.ExecContext(c.UserContext(), `
		UPDATE students SET nama = $1, prodi = $2, angkatan = $3, ipk_terakhir = $4
		WHERE id = $5 AND deleted_at IS NULL`,
		strings.TrimSpace(input.Nama), strings.TrimSpace(input.Prodi),
		input.Angkatan, input.IPKTerakhir, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	return c.JSON(response{Success: true, Message: "Data mahasiswa berhasil diperbarui"})
}

func (api *API) deleteStudent(c *fiber.Ctx) error {
	if err := api.requireAdmin(c); err != nil {
		return err
	}
	id, err := parseID(c.Params("id"))
	if err != nil {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	result, err := api.db.ExecContext(c.UserContext(),
		"UPDATE students SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fail(c, fiber.StatusNotFound, "Mahasiswa tidak ditemukan")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func queryInt(c *fiber.Ctx, key string, fallback, min, max int) (int, error) {
	value := c.Query(key)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < min || number > max {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return number, nil
}

func creditLimit(ipk float64) int {
	switch {
	case ipk >= 3:
		return 24
	case ipk >= 2.5:
		return 21
	default:
		return 18
	}
}
