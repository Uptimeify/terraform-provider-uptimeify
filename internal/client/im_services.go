package client

import (
	"context"
	"net/http"
	"net/url"
)

// IM service catalog (server/lib/api-contract/im-services.ts, migration 0313).

// IMServiceMonitor is one monitor of a service; ids are unique per type only. MonitorName is
// answered by the API and ignored on write.
type IMServiceMonitor struct {
	MonitorType string `json:"monitorType"`
	MonitorID   int64  `json:"monitorId"`
}

// IMService is the answer of GET, POST and PATCH /api/im/services.
type IMService struct {
	ID             int64              `json:"id"`
	PublicID       string             `json:"publicId"`
	OrganizationID int64              `json:"organizationId"`
	Name           string             `json:"name"`
	Description    *string            `json:"description"`
	Criticality    string             `json:"criticality"`
	OwnerTeamID    *int64             `json:"ownerTeamId"`
	Monitors       []IMServiceMonitor `json:"monitors"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
}

// GetIMService takes the numeric id or the public id.
func (c *Client) GetIMService(ctx context.Context, id string) (*IMService, error) {
	var s IMService
	return &s, c.Do(ctx, http.MethodGet, imPath("services/%s", url.PathEscape(id)), nil, &s)
}

func (c *Client) CreateIMService(ctx context.Context, body map[string]any) (*IMService, error) {
	var s IMService
	return &s, c.Do(ctx, http.MethodPost, imPath("services"), body, &s)
}

// UpdateIMService is a merge patch; "monitors", when given, replaces the whole set.
func (c *Client) UpdateIMService(ctx context.Context, id string, patch map[string]any) (*IMService, error) {
	var s IMService
	return &s, c.Do(ctx, http.MethodPatch, imPath("services/%s", url.PathEscape(id)), patch, &s)
}

func (c *Client) DeleteIMService(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, imPath("services/%s", url.PathEscape(id)), nil, nil)
}
