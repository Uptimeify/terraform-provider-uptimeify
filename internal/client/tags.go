package client

import (
	"context"
	"net/http"
	"net/url"
)

// TagColors is TAG_PALETTE from monitoring-app/server/lib/tags/palette.ts.
var TagColors = []string{"slate", "red", "amber", "green", "teal", "blue", "indigo", "violet", "pink", "gray"}

// Tag is the tag row of the API contract (server/lib/api-contract/tags.ts).
type Tag struct {
	ID         int64  `json:"id"`
	PublicID   string `json:"publicId"`
	CustomerID *int64 `json:"customerId"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// TagCreate is the POST /api/tags body.
type TagCreate struct {
	Name       string `json:"name"`
	Color      string `json:"color"`
	CustomerID *int64 `json:"customerId,omitempty"`
}

// ListTags reads GET /api/tags. It is a catalog (bare array, api-standards §9), no envelope.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var tags []Tag
	return tags, c.Do(ctx, http.MethodGet, "/api/tags", nil, &tags)
}

// GetTag reads one tag by numeric id or publicId.
func (c *Client) GetTag(ctx context.Context, id string) (*Tag, error) {
	var t Tag
	return &t, c.Do(ctx, http.MethodGet, "/api/tags/"+url.PathEscape(id), nil, &t)
}

func (c *Client) CreateTag(ctx context.Context, in TagCreate) (*Tag, error) {
	var t Tag
	return &t, c.Do(ctx, http.MethodPost, "/api/tags", in, &t)
}

// UpdateTag sends only the keys in patch; "customerId": nil makes the tag organization-wide.
func (c *Client) UpdateTag(ctx context.Context, id string, patch map[string]any) (*Tag, error) {
	var t Tag
	return &t, c.Do(ctx, http.MethodPatch, "/api/tags/"+url.PathEscape(id), patch, &t)
}

func (c *Client) DeleteTag(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/tags/"+url.PathEscape(id), nil, nil)
}
