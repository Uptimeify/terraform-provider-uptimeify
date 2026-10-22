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

// ListCustomers reads every customer in scope: with fields=minimal the envelope form is one page
// with every customer, sorted by name.
func (c *Client) ListCustomers(ctx context.Context) ([]Customer, error) {
	var env itemsEnvelope[Customer]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/customers?envelope=items&fields=minimal", nil, &env)
}
