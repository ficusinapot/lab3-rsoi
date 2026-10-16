package bookings

import (
	"log/slog"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
	"lab2/platform/rest"
)

const reservationsTag = "Reservations"

func Register(api huma.API, service coreifc.Bookings, logger *slog.Logger, cfg rest.Config) {
	registerList(api, service, logger)
	registerGet(api, service, logger)
	registerCreate(api, service, logger, cfg)
	registerCancel(api, service, logger)
}
