package hotels

import (
	"log/slog"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
)

const hotelsTag = "Hotels"

func Register(api huma.API, service coreifc.Hotels, logger *slog.Logger) {
	registerList(api, service, logger)
	registerGet(api, service, logger)
}
