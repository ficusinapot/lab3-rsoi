package hotels

import (
	"context"
	"log/slog"
	"net/http"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
	"lab2/reservation/internal/models/entities"
	"lab2/reservation/internal/rest/response"
)

func registerList(api huma.API, service coreifc.Hotels, logger *slog.Logger) {
	huma.Register(api, huma.Operation{
		OperationID: "list-hotels",
		Method:      http.MethodGet,
		Path:        "/hotels",
		Tags:        []string{hotelsTag},
		Errors:      []int{http.StatusBadRequest, http.StatusInternalServerError},
		Parameters: []*huma.Param{
			{Name: "page", In: "query", Schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64"}},
			{Name: "size", In: "query", Schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64"}},
		},
	}, func(ctx context.Context, input *ListInput) (*ListOutput, error) {
		page, err := service.List(ctx, entities.PageQuery{
			Page: input.Page.ToPointer(), Size: input.Size.ToPointer(),
		})
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		return &ListOutput{Body: pageResponse(page)}, nil
	})
}
