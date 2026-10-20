package client

import (
	"context"
	"net/http"
	"net/url"
)

// MaintenanceTargetTypes is MAINTENANCE_TARGET_TYPES from monitoring-app/server/lib/maintenance/write-schemas.ts.
var MaintenanceTargetTypes = []string{"website", "dns", "icmp", "smtp", "ssh", "tcp", "ftp", "imap_pop", "customer_domain", "customer_ip"}

// MaintenanceTarget is one entry of `targets` (request: type + id; response adds name and publicId).
type MaintenanceTarget struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	Name     string `json:"name,omitempty"`
	PublicID string `json:"publicId,omitempty"`
}

// RecurrencePattern is the request/response pattern. Only the successor field intervalCount;
// the deprecated `interval` twin is never sent and ignored on read.
type RecurrencePattern struct {
	Frequency         string  `json:"frequency"`
	IntervalCount     *int64  `json:"intervalCount,omitempty"`
	DaysOfWeek        []int64 `json:"daysOfWeek,omitempty"`
	DayOfMonth        *int64  `json:"dayOfMonth,omitempty"`
	EndRecurrenceDate *string `json:"endRecurrenceDate,omitempty"`
}

// MaintenanceWindow is GET /api/maintenance-windows/{id} (server/lib/api-contract/maintenance-windows.ts),
// without the legacy single-target id fields and the deprecated `website` row.
type MaintenanceWindow struct {
	ID                int64               `json:"id"`
	PublicID          string              `json:"publicId"`
	CustomerID        *int64              `json:"customerId"`
	OrganizationID    *int64              `json:"organizationId"`
	Name              string              `json:"name"`
	Description       *string             `json:"description"`
	StartTime         string              `json:"startTime"`
	EndTime           string              `json:"endTime"`
	IsRecurring       bool                `json:"isRecurring"`
	RecurrencePattern *RecurrencePattern  `json:"recurrencePattern"`
	IsActive          bool                `json:"isActive"`
	Timezone          string              `json:"timezone"`
	CreatedAt         string              `json:"createdAt"`
	UpdatedAt         string              `json:"updatedAt"`
	Targets           []MaintenanceTarget `json:"targets"`
	Tags              []Tag               `json:"tags"`
}

// MaintenanceWindowCreate is the POST body: targets and/or tagIds, or customerId alone.
type MaintenanceWindowCreate struct {
	Name              string              `json:"name"`
	Description       *string             `json:"description,omitempty"`
	StartTime         string              `json:"startTime"`
	EndTime           string              `json:"endTime"`
	IsRecurring       bool                `json:"isRecurring"`
	RecurrencePattern *RecurrencePattern  `json:"recurrencePattern,omitempty"`
	IsActive          bool                `json:"isActive"`
	Timezone          string              `json:"timezone,omitempty"`
	CustomerID        *int64              `json:"customerId,omitempty"`
	Targets           []MaintenanceTarget `json:"targets,omitempty"`
	TagIDs            []int64             `json:"tagIds,omitempty"`
}

func (c *Client) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	var w MaintenanceWindow
	return &w, c.Do(ctx, http.MethodGet, "/api/maintenance-windows/"+url.PathEscape(id), nil, &w)
}

// CreateMaintenanceWindow returns the bare row (no targets/tags); callers read it back with GET.
func (c *Client) CreateMaintenanceWindow(ctx context.Context, in MaintenanceWindowCreate) (*MaintenanceWindow, error) {
	var w MaintenanceWindow
	return &w, c.Do(ctx, http.MethodPost, "/api/maintenance-windows", in, &w)
}

// UpdateMaintenanceWindow sends only the keys in patch. The answer has no targets/tags.
func (c *Client) UpdateMaintenanceWindow(ctx context.Context, id string, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, "/api/maintenance-windows/"+url.PathEscape(id), patch, nil)
}

func (c *Client) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/maintenance-windows/"+url.PathEscape(id), nil, nil)
}
