package bookings

import (
	"strings"
	"unicode/utf8"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
)

func validateUser(name string) error {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return oops.Wrap(entities.ErrInvalidInput)
	}
	return nil
}
