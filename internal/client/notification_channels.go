package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// NotificationChannelTypes is NOTIFICATION_CHANNEL_TYPES (server/lib/notification-channel-types.ts).
var NotificationChannelTypes = []string{"email", "sms", "webhook", "incident_management", "slack", "discord", "teams",
	"pagerduty", "pushover", "opsgenie", "matrix", "jira", "allquiet", "telegram", "googlechat", "mattermost",
	"rocketchat", "ntfy", "gotify", "ilert", "grafanaoncall", "incidentio", "squadcast", "github", "gitlab",
	"linear", "servicenow", "lark", "dingtalk", "wecom"}

// ChannelSecret is one SECRET_FIELDS descriptor (server/lib/notification-channel-secrets.ts):
// the config key holding the secret and the has-flag the API answers instead.
type ChannelSecret struct{ Key, HasKey string }

var chatWebhookURL = []ChannelSecret{{"webhookUrl", "hasWebhookUrl"}, {"url", "hasWebhookUrl"}}

// ChannelSecrets maps a channel type to its secret config keys. The API stores them encrypted and
// answers them as null plus a has<Secret> flag.
var ChannelSecrets = map[string][]ChannelSecret{
	"webhook":       {{"secret", "hasSecret"}},
	"slack":         append([]ChannelSecret{{"secret", "hasSecret"}}, chatWebhookURL...),
	"discord":       append([]ChannelSecret{{"secret", "hasSecret"}}, chatWebhookURL...),
	"teams":         chatWebhookURL,
	"pagerduty":     {{"routingKey", "hasRoutingKey"}},
	"pushover":      {{"userKey", "hasUserKey"}, {"apiToken", "hasApiToken"}},
	"opsgenie":      {{"apiKey", "hasApiKey"}},
	"matrix":        {{"accessToken", "hasAccessToken"}},
	"jira":          {{"apiToken", "hasApiToken"}},
	"github":        {{"token", "hasToken"}},
	"gitlab":        {{"token", "hasToken"}},
	"linear":        {{"apiKey", "hasApiKey"}},
	"servicenow":    {{"password", "hasPassword"}},
	"allquiet":      {{"webhookUrl", "hasWebhookUrl"}},
	"telegram":      {{"botToken", "hasBotToken"}},
	"googlechat":    {{"webhookUrl", "hasWebhookUrl"}},
	"mattermost":    {{"webhookUrl", "hasWebhookUrl"}},
	"rocketchat":    {{"webhookUrl", "hasWebhookUrl"}},
	"lark":          {{"webhookUrl", "hasWebhookUrl"}},
	"dingtalk":      {{"webhookUrl", "hasWebhookUrl"}},
	"wecom":         {{"webhookUrl", "hasWebhookUrl"}},
	"ntfy":          {{"accessToken", "hasAccessToken"}},
	"gotify":        {{"appToken", "hasAppToken"}},
	"ilert":         {{"integrationKey", "hasIntegrationKey"}},
	"grafanaoncall": {{"webhookUrl", "hasWebhookUrl"}},
	"incidentio":    {{"token", "hasToken"}},
	"squadcast":     {{"webhookUrl", "hasWebhookUrl"}},
}

// NotificationChannel is the channel row of notification-channels.ts (secrets redacted).
type NotificationChannel struct {
	ID                  int64           `json:"id"`
	OrganizationID      int64           `json:"organizationId"`
	CustomerID          *int64          `json:"customerId"`
	WebsiteID           *int64          `json:"websiteId"`
	Type                string          `json:"type"`
	Category            string          `json:"category"`
	Name                string          `json:"name"`
	Config              map[string]any  `json:"config"`
	DelaySeconds        *int64          `json:"delaySeconds"`
	Conditions          json.RawMessage `json:"conditions"`
	AllowedPackageTypes []string        `json:"allowedPackageTypes"`
	IsActive            bool            `json:"isActive"`
	CreatedAt           string          `json:"createdAt"`
}

func (c *Client) CreateNotificationChannel(ctx context.Context, body map[string]any) (*NotificationChannel, error) {
	var ch NotificationChannel
	return &ch, c.Do(ctx, http.MethodPost, "/api/notification-channels", body, &ch)
}

func (c *Client) GetNotificationChannel(ctx context.Context, id string) (*NotificationChannel, error) {
	var ch NotificationChannel
	return &ch, c.Do(ctx, http.MethodGet, "/api/notification-channels/"+url.PathEscape(id), nil, &ch)
}

// ListNotificationChannels reads the organization channels, plus the customer's when customerID is set.
func (c *Client) ListNotificationChannels(ctx context.Context, customerID string) ([]NotificationChannel, error) {
	p := "/api/notification-channels?envelope=items"
	if customerID != "" {
		p += "&customerId=" + url.QueryEscape(customerID)
	}
	var out struct {
		Items []NotificationChannel `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, p, nil, &out)
}

func (c *Client) UpdateNotificationChannel(ctx context.Context, id string, patch map[string]any) (*NotificationChannel, error) {
	var ch NotificationChannel
	return &ch, c.Do(ctx, http.MethodPatch, "/api/notification-channels/"+url.PathEscape(id), patch, &ch)
}

func (c *Client) DeleteNotificationChannel(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/notification-channels/"+url.PathEscape(id), nil, nil)
}
