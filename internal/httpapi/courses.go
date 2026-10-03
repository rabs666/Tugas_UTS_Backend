package httpapi

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (api *API) listCourses(c *fiber.Ctx) error {
	where := []string{"TRUE"}
	args := []interface{}{}
	if semester := c.Query("semester"); semester != "" {
		number, err := queryInt(c, "semester", 1, 1, 14)
		if err != nil {
			return fail(c, fiber.StatusUnprocessableEntity, "Parameter semester tidak valid")
		}
		args = append(args, number)
		where = append(where, fmt.Sprintf("c.semester = $%d", len(args)))
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(c.kode_mk ILIKE $%d OR c.nama_mk ILIKE $%d)", len(args), len(args)))
	}
	if available := c.Query("available"); available != "" {
		if available != "true" && available != "false" {
			return fail(c, fiber.StatusUnprocessableEntity, "Parameter available harus true atau false")
		}
		if available == "true" {
			where = append(where, "COALESCE(enrolled.terisi, 0) < c.kuota")
		}
	}
	rows, err := api.db.QueryContext(c.UserContext(), `
		SELECT c.id, c.kode_mk, c.nama_mk, c.sks, c.semester, c.kuota,
		       COALESCE(enrolled.terisi, 0) AS terisi,
		       GREATEST(c.kuota - COALESCE(enrolled.terisi, 0), 0) AS sisa_kuota
		FROM courses c
		LEFT JOIN (
			SELECT course_id, COUNT(*) AS terisi FROM enrollments GROUP BY course_id
		) enrolled ON enrolled.course_id = c.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.semester, c.kode_mk`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	data := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var code, name string
		var sks, semester, quota, filled, remaining int
		if err := rows.Scan(&id, &code, &name, &sks, &semester, &quota, &filled, &remaining); err != nil {
			return err
		}
		data = append(data, map[string]interface{}{
			"id": id, "kode_mk": code, "nama_mk": name, "sks": sks,
			"semester": semester, "kuota": quota, "terisi": filled, "sisa_kuota": remaining,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return c.JSON(response{Success: true, Message: "Data mata kuliah berhasil diambil", Data: data})
}
