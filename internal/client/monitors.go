package client

import (
	"context"
	"net/http"
	"net/url"
)

// MonitorRow is the row shared by the seven protocol monitor types
// (server/lib/api-contract/protocol-monitors.ts: `base` plus port/checkMode/config).
// Secrets in Config are always null in answers.
type MonitorRow struct {
	ID                       int64          `json:"id"`
	PublicID                 string         `json:"publicId"`
	OrganizationID           int64          `json:"organizationId"`
	CustomerID               int64          `json:"customerId"`
	Name                     string         `json:"name"`
	Hostname                 string         `json:"hostname"`
	CheckInterval            int64          `json:"checkInterval"`
	TimeoutSeconds           int64          `json:"timeoutSeconds"`
	Status                   string         `json:"status"`
	ManagementType           string         `json:"managementType"`
	AllowedCheckCountryCodes []string       `json:"allowedCheckCountryCodes"`
	CreatedAt                string         `json:"createdAt"`
	UpdatedAt                string         `json:"updatedAt"`
	Port                     *int64         `json:"port"`
	CheckMode                string         `json:"checkMode"`
	Config                   map[string]any `json:"config"`
	DNSConfig                *DNSConfig     `json:"dnsConfig"`
}

// DNSConfig is dnsConfigView of the contract (stored normalized: upper-case types, trimmed values).
type DNSConfig struct {
	RRTypes        []string            `json:"rrtypes"`
	MatchMode      string              `json:"matchMode"`
	ExpectedValues map[string][]string `json:"expectedValues"`
	TriggerOn      *struct {
		ResolveError *bool `json:"resolveError"`
		Mismatch     *bool `json:"mismatch"`
	} `json:"triggerOn"`
	RequireExpectedValues *bool `json:"requireExpectedValues"`
}

// Monitor endpoints live under /api/<kind>, kind e.g. "dns-monitors". id is the numeric id or publicId.

func (c *Client) GetMonitor(ctx context.Context, kind, id string) (*MonitorRow, error) {
	var m MonitorRow
	return &m, c.Do(ctx, http.MethodGet, "/api/"+kind+"/"+url.PathEscape(id), nil, &m)
}

func (c *Client) CreateMonitor(ctx context.Context, kind string, body map[string]any) (*MonitorRow, error) {
	var m MonitorRow
	return &m, c.Do(ctx, http.MethodPost, "/api/"+kind, body, &m)
}

// UpdateMonitor sends only the keys in patch.
func (c *Client) UpdateMonitor(ctx context.Context, kind, id string, patch map[string]any) (*MonitorRow, error) {
	var m MonitorRow
	return &m, c.Do(ctx, http.MethodPatch, "/api/"+kind+"/"+url.PathEscape(id), patch, &m)
}

func (c *Client) DeleteMonitor(ctx context.Context, kind, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/"+kind+"/"+url.PathEscape(id), nil, nil)
}
