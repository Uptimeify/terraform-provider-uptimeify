package client

import (
	"context"
	"net/http"
	"net/url"
)

// Customer is the short customer form (GET /api/customers?fields=minimal, and the matching
// subset of GET /api/customers/{id}).
type Customer struct {
	ID       int64  `json:"id"`
	PublicID string `json:"publicId"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

func (c *Client) GetCustomer(ctx context.Context, id string) (*Customer, error) {
	var cu Customer
	return &cu, c.Do(ctx, http.MethodGet, "/api/customers/"+url.PathEscape(id), nil, &cu)
}

// ListCustomers reads every customer in scope. GET /api/customers has no envelope form; without
// `page` and with fields=minimal it answers a bare array sorted by name.
func (c *Client) ListCustomers(ctx context.Context) ([]Customer, error) {
	var cs []Customer
	return cs, c.Do(ctx, http.MethodGet, "/api/customers?fields=minimal", nil, &cs)
}
