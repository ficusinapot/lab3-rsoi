package hotels

import (
	"encoding/json"
	"testing"
	"uuid"

	"lab2/reservation/internal/models/entities"
)

func TestPageResponse(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	body, err := json.Marshal(pageResponse(entities.Page{
		Page: 1, PageSize: 10, TotalElements: 1, Items: []entities.Hotel{{
			HotelUID: id, Name: "Hotel", Price: 100,
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Page  int `json:"page"`
		Items []struct {
			HotelUID string `json:"hotelUid"`
			Name     string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Page != 1 || len(got.Items) != 1 || got.Items[0].HotelUID != id.String() ||
		got.Items[0].Name != "Hotel" {
		t.Fatalf("response: %s", body)
	}
}
