package client

import (
	"context"
	"net/http"
	"net/url"
)

// Incident Management: alert sources, routing rules and org settings
// (server/lib/api-contract/im-sources.ts, im-routing.ts, im-channels.ts).

// ImSeverities is imSeverity of the API contract.
var ImSeverities = []string{"sev1", "sev2", "sev3", "sev4"}

// Endpoint is the base URL the client talks to (ingest URLs are built on it).
func (c *Client) Endpoint() string { return c.endpoint }

// Slices and maps are pointers so "absent" (nil) and "empty" survive the JSON round trip:
// the API stores mappings verbatim.

type PMEntry struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type PMStep struct {
	Kind     string     `json:"kind"`
	Type     *string    `json:"type,omitempty"`
	Expr     *string    `json:"expr,omitempty"`
	Entries  *[]PMEntry `json:"entries,omitempty"`
	Fallback *string    `json:"fallback,omitempty"`
}

type PMFlags struct {
	Grouped *bool `json:"grouped,omitempty"`
	Hidden  *bool `json:"hidden,omitempty"`
	Image   *bool `json:"image,omitempty"`
	Expand  *bool `json:"expand,omitempty"`
}

type PMAttribute struct {
	Key      string    `json:"key"`
	Standard *bool     `json:"standard,omitempty"`
	Steps    *[]PMStep `json:"steps,omitempty"`
	Flags    *PMFlags  `json:"flags,omitempty"`
}

// PayloadMapping is the union of the v1 field mapping and the v2 attribute list.
type PayloadMapping struct {
	Version         *int               `json:"version,omitempty"`
	Attributes      *[]PMAttribute     `json:"attributes,omitempty"`
	Title           *string            `json:"title,omitempty"`
	Severity        *string            `json:"severity,omitempty"`
	Status          *string            `json:"status,omitempty"`
	Host            *string            `json:"host,omitempty"`
	URL             *string            `json:"url,omitempty"`
	DedupKey        *string            `json:"dedupKey,omitempty"`
	SeverityMap     *map[string]string `json:"severityMap,omitempty"`
	ResolveValues   *[]string          `json:"resolveValues,omitempty"`
	Custom          *map[string]string `json:"custom,omitempty"`
	DefaultSeverity *string            `json:"defaultSeverity,omitempty"`
	EmailSelectors  *map[string]string `json:"emailSelectors,omitempty"`
}

type AuthToggle struct {
	Enabled bool `json:"enabled"`
}

// AuthMode reads only the camelCase successor urlToken (url_token is its deprecated twin).
type AuthMode struct {
	URLToken *bool       `json:"urlToken,omitempty"`
	HMAC     *AuthToggle `json:"hmac,omitempty"`
	Bearer   *AuthToggle `json:"bearer,omitempty"`
}

type MaintenanceWindowIM struct {
	Dow      *[]int64 `json:"dow,omitempty"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Timezone *string  `json:"timezone,omitempty"`
}

type SourceMaintenance struct {
	Windows []MaintenanceWindowIM `json:"windows"`
}

type AlertSource struct {
	ID                    int64              `json:"id"`
	OrganizationID        int64              `json:"organizationId"`
	TeamID                int64              `json:"teamId"`
	Name                  string             `json:"name"`
	Type                  string             `json:"type"`
	SecondaryExpiresAt    *string            `json:"secondaryExpiresAt"`
	AuthMode              AuthMode           `json:"authMode"`
	PayloadMapping        PayloadMapping     `json:"payloadMapping"`
	AutoResolve           bool               `json:"autoResolve"`
	GroupingWindowMinutes *int64             `json:"groupingWindowMinutes"`
	Maintenance           *SourceMaintenance `json:"maintenance"`
	CreatedAt             string             `json:"createdAt"`
	UpdatedAt             string             `json:"updatedAt"`
	// Create answer only.
	IngestToken         *string `json:"ingestToken,omitempty"`
	InboundEmailAddress *string `json:"inboundEmailAddress,omitempty"`
}

type itemsEnvelope[T any] struct {
	Items []T `json:"items"`
}

func (c *Client) ListAlertSources(ctx context.Context) ([]AlertSource, error) {
	var env itemsEnvelope[AlertSource]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/im/sources?envelope=items", nil, &env)
}

func (c *Client) GetAlertSource(ctx context.Context, id string) (*AlertSource, error) {
	var s AlertSource
	return &s, c.Do(ctx, http.MethodGet, "/api/im/sources/"+url.PathEscape(id), nil, &s)
}

func (c *Client) CreateAlertSource(ctx context.Context, body map[string]any) (*AlertSource, error) {
	var s AlertSource
	return &s, c.Do(ctx, http.MethodPost, "/api/im/sources", body, &s)
}

func (c *Client) UpdateAlertSource(ctx context.Context, id string, patch map[string]any) (*AlertSource, error) {
	var s AlertSource
	return &s, c.Do(ctx, http.MethodPatch, "/api/im/sources/"+url.PathEscape(id), patch, &s)
}

func (c *Client) DeleteAlertSource(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/im/sources/"+url.PathEscape(id), nil, nil)
}

// RotateIngestToken mints a new ingest token; the old one works until secondaryExpiresAt.
func (c *Client) RotateIngestToken(ctx context.Context, id string) (string, error) {
	var out struct {
		IngestToken string `json:"ingestToken"`
	}
	return out.IngestToken, c.Do(ctx, http.MethodPost, "/api/im/sources/"+url.PathEscape(id)+"/rotate-token", nil, &out)
}

// SetHMACSecret stores secret ("" = server-generated) and enables HMAC.
func (c *Client) SetHMACSecret(ctx context.Context, id, secret string) error {
	body := map[string]any{}
	if secret != "" {
		body["secret"] = secret
	}
	return c.Do(ctx, http.MethodPost, "/api/im/sources/"+url.PathEscape(id)+"/set-hmac-secret", body, nil)
}

// RotateBearerToken mints a bearer token (first time or rotation) and enables bearer auth.
func (c *Client) RotateBearerToken(ctx context.Context, id string) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	return out.Token, c.Do(ctx, http.MethodPost, "/api/im/sources/"+url.PathEscape(id)+"/rotate-bearer-token", nil, &out)
}

// RoutingMatch reads only the camelCase successors (snake_case keys are deprecated twins).
type RoutingMatch struct {
	SourceIDs    *[]int64           `json:"sourceIds,omitempty"`
	CustomerIDs  *[]int64           `json:"customerIds,omitempty"`
	MonitorIDs   *[]int64           `json:"monitorIds,omitempty"`
	Tags         *[]string          `json:"tags,omitempty"`
	Severity     *[]string          `json:"severity,omitempty"`
	FieldMatches *map[string]string `json:"fieldMatches,omitempty"`
}

type RoutingRule struct {
	ID               int64         `json:"id"`
	OrganizationID   int64         `json:"organizationId"`
	Priority         int64         `json:"priority"`
	Match            *RoutingMatch `json:"match"`
	TeamID           int64         `json:"teamId"`
	SeverityOverride *string       `json:"severityOverride"`
	CreatedAt        string        `json:"createdAt"`
	UpdatedAt        string        `json:"updatedAt"`
}

func (c *Client) GetRoutingRule(ctx context.Context, id string) (*RoutingRule, error) {
	var r RoutingRule
	return &r, c.Do(ctx, http.MethodGet, "/api/im/routing/"+url.PathEscape(id), nil, &r)
}

func (c *Client) CreateRoutingRule(ctx context.Context, body map[string]any) (*RoutingRule, error) {
	var r RoutingRule
	return &r, c.Do(ctx, http.MethodPost, "/api/im/routing", body, &r)
}

func (c *Client) UpdateRoutingRule(ctx context.Context, id string, patch map[string]any) (*RoutingRule, error) {
	var r RoutingRule
	return &r, c.Do(ctx, http.MethodPatch, "/api/im/routing/"+url.PathEscape(id), patch, &r)
}

func (c *Client) DeleteRoutingRule(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/im/routing/"+url.PathEscape(id), nil, nil)
}

type ImOrgSettings struct {
	OrganizationID    int64  `json:"organizationId"`
	Enabled           bool   `json:"enabled"`
	OverageEnabled    bool   `json:"overageEnabled"`
	MonitoringBridge  string `json:"monitoringBridge"`
	FallbackChannelID *int64 `json:"fallbackChannelId"`
}

func (c *Client) GetImOrgSettings(ctx context.Context) (*ImOrgSettings, error) {
	var s ImOrgSettings
	return &s, c.Do(ctx, http.MethodGet, "/api/im/org-settings", nil, &s)
}

func (c *Client) UpdateImOrgSettings(ctx context.Context, patch map[string]any) (*ImOrgSettings, error) {
	var s ImOrgSettings
	return &s, c.Do(ctx, http.MethodPatch, "/api/im/org-settings", patch, &s)
}
