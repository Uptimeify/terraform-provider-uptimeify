package client

import (
	"context"
	"net/http"
)

// LocationCountry is one entry of GET /api/monitoring-locations/countries (api-contract/general.ts).
type LocationCountry struct {
	Code      string `json:"code"`
	Locations int64  `json:"locations"`
	Workers   int64  `json:"workers"`
}

// ListLocationCountries returns the countries with at least one active check worker, by code.
func (c *Client) ListLocationCountries(ctx context.Context) ([]LocationCountry, error) {
	var out struct {
		Countries []LocationCountry `json:"countries"`
	}
	return out.Countries, c.Do(ctx, http.MethodGet, "/api/monitoring-locations/countries", nil, &out)
}
