package bookings

import (
	"log/slog"
	"mime"
	"net/http"

	"lab2/gateway/internal/models/coreifc"
	"lab2/gateway/internal/rest/bookings/api"
	"lab2/platform/rest"

	"github.com/go-chi/chi/v5"
)

type handler struct {
	commands coreifc.BookingCommands
	queries  coreifc.BookingQueries
	logger   *slog.Logger
}

var _ api.StrictServerInterface = (*handler)(nil)

func Register(
	router chi.Router, commands coreifc.BookingCommands, queries coreifc.BookingQueries,
	logger *slog.Logger, cfg rest.Config,
) {
	h := &handler{commands: commands, queries: queries, logger: logger}
	strictHandler := api.NewStrictHandlerWithOptions(h, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			h.writeValidationError(w, r, "validation failed", err)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			h.writeServiceError(w, r, err)
		},
	})
	requestBodyLimits := api.MiddlewareFunc(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mediaType != "application/json" {
					h.writeValidationError(w, r, "invalid content type", nil)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
			next.ServeHTTP(w, r)
		})
	})
	api.HandlerWithOptions(strictHandler, api.ChiServerOptions{
		BaseRouter:  router,
		Middlewares: []api.MiddlewareFunc{requestBodyLimits},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			h.writeValidationError(w, r, "validation failed", err)
		},
	})
}
