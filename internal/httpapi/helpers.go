package httpapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
)

func uniqueConflict(c *fiber.Ctx, err error, field, message string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return invalid(c, map[string][]string{field: {message}})
	}
	return err
}
