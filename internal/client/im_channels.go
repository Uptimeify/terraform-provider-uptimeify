package client

import (
	"context"
	"net/http"
	"net/url"
)

// Incident Management paging channels, outbound integrations and on-call
// (server/lib/api-contract/im-channels.ts, im-outbound.ts, im-reporting.ts).

// ImChannelTypes is channelType of the im-channels contract.
var ImChannelTypes = []string{"webhook", "email", "slack"}

// ImOutboundTypes and ImOutboundEventKinds are type and eventKind of the im-outbound contract.
var (
	ImOutboundTypes      = []string{"slack", "teams", "discord", "jira", "pagerduty_v2", "opsgenie", "webhook"}
	ImOutboundEventKinds = []string{"incident_created", "acknowledged", "severity_changed", "status_changed", "comment", "resolved"}
)

// ImChannelConfig is the read form: secrets are null, has* flags tell whether they are stored,
// headers carries the header names only.
type ImChannelConfig struct {
	HasWebhookURL bool               `json:"hasWebhookUrl"`
	Headers       map[string]*string `json:"headers"`
	Email         *string            `json:"email"`
}

type ImChannel struct {
	ID             int64           `json:"id"`
	OrganizationID int64           `json:"organizationId"`
	TeamID         *int64          `json:"teamId"`
	Type           string          `json:"type"`
	Name           string          `json:"name"`
	Config         ImChannelConfig `json:"config"`
	IsActive       bool            `json:"isActive"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

func (c *Client) ListImChannels(ctx context.Context) ([]ImChannel, error) {
	var env itemsEnvelope[ImChannel]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/im/channels?envelope=items", nil, &env)
}

func (c *Client) GetImChannel(ctx context.Context, id string) (*ImChannel, error) {
	var ch ImChannel
	return &ch, c.Do(ctx, http.MethodGet, "/api/im/channels/"+url.PathEscape(id), nil, &ch)
}

func (c *Client) CreateImChannel(ctx context.Context, body map[string]any) (*ImChannel, error) {
	var ch ImChannel
	return &ch, c.Do(ctx, http.MethodPost, "/api/im/channels", body, &ch)
}

func (c *Client) UpdateImChannel(ctx context.Context, id string, patch map[string]any) (*ImChannel, error) {
	var ch ImChannel
	return &ch, c.Do(ctx, http.MethodPatch, "/api/im/channels/"+url.PathEscape(id), patch, &ch)
}

func (c *Client) DeleteImChannel(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/im/channels/"+url.PathEscape(id), nil, nil)
}

// ImOutboundConfig is the read form of an outbound config (secrets null, has* flags).
type ImOutboundConfig struct {
	HasWebhookURL  bool    `json:"hasWebhookUrl"`
	HasBearerToken bool    `json:"hasBearerToken"`
	HasHeaders     bool    `json:"hasHeaders"`
	HasAPIToken    bool    `json:"hasApiToken"`
	HasRoutingKey  bool    `json:"hasRoutingKey"`
	HasAPIKey      bool    `json:"hasApiKey"`
	Email          *string `json:"email"`
	BaseURL        *string `json:"baseUrl"`
	ProjectKey     *string `json:"projectKey"`
}

// ImOutboundFilters reads only the camelCase successor eventKinds (event_kinds is its deprecated twin).
type ImOutboundFilters struct {
	Teams      []int64  `json:"teams"`
	Severities []string `json:"severities"`
	EventKinds []string `json:"eventKinds"`
}

type ImOutbound struct {
	ID             int64             `json:"id"`
	OrganizationID int64             `json:"organizationId"`
	TeamID         *int64            `json:"teamId"`
	Type           string            `json:"type"`
	Name           string            `json:"name"`
	Config         ImOutboundConfig  `json:"config"`
	Filters        ImOutboundFilters `json:"filters"`
	IsActive       bool              `json:"isActive"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

func (c *Client) ListImOutbound(ctx context.Context) ([]ImOutbound, error) {
	var env itemsEnvelope[ImOutbound]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/im/outbound?envelope=items", nil, &env)
}

func (c *Client) GetImOutbound(ctx context.Context, id string) (*ImOutbound, error) {
	var o ImOutbound
	return &o, c.Do(ctx, http.MethodGet, "/api/im/outbound/"+url.PathEscape(id), nil, &o)
}

func (c *Client) CreateImOutbound(ctx context.Context, body map[string]any) (*ImOutbound, error) {
	var o ImOutbound
	return &o, c.Do(ctx, http.MethodPost, "/api/im/outbound", body, &o)
}

func (c *Client) UpdateImOutbound(ctx context.Context, id string, patch map[string]any) (*ImOutbound, error) {
	var o ImOutbound
	return &o, c.Do(ctx, http.MethodPatch, "/api/im/outbound/"+url.PathEscape(id), patch, &o)
}

func (c *Client) DeleteImOutbound(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/im/outbound/"+url.PathEscape(id), nil, nil)
}

// ImOnCallNow is one row of GET /api/im/on-call (one per schedule).
type ImOnCallNow struct {
	TeamID      int64   `json:"teamId"`
	ScheduleID  int64   `json:"scheduleId"`
	UserID      string  `json:"userId"`
	UserName    *string `json:"userName"`
	UserImage   *string `json:"userImage"`
	ShiftEndsAt string  `json:"shiftEndsAt"`
}

type ImOnCallUser struct {
	UserID    string  `json:"userId"`
	UserName  *string `json:"userName"`
	UserImage *string `json:"userImage"`
}

// ImOnCallTier is one row of GET /api/im/teams/{id}/on-call-now.
type ImOnCallTier struct {
	TierID         int64          `json:"tierId"`
	TierOrder      int64          `json:"tierOrder"`
	Users          []ImOnCallUser `json:"users"`
	UncoveredSince *string        `json:"uncoveredSince"`
}

func (c *Client) ListImOnCall(ctx context.Context) ([]ImOnCallNow, error) {
	var env itemsEnvelope[ImOnCallNow]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/im/on-call?envelope=items", nil, &env)
}

func (c *Client) ListImTeamOnCallNow(ctx context.Context, teamID int64) ([]ImOnCallTier, error) {
	var env itemsEnvelope[ImOnCallTier]
	return env.Items, c.Do(ctx, http.MethodGet, imPath("teams/%d/on-call-now?envelope=items", teamID), nil, &env)
}
