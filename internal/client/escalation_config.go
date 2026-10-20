package client

import (
	"context"
	"net/http"
	"strconv"
)

// EscalationConfig is the organization's escalation webhook (notification-channels.ts escalationConfig).
// ID is nil while nothing is saved (GET then answers the inactive defaults).
type EscalationConfig struct {
	ID                   *int64            `json:"id"`
	OrganizationID       int64             `json:"organizationId"`
	WebhookURL           *string           `json:"webhookUrl"`
	WebhookMethod        *string           `json:"webhookMethod"`
	WebhookHeaders       map[string]string `json:"webhookHeaders"`
	WebhookBodyTemplate  *string           `json:"webhookBodyTemplate"`
	WebhookTimeout       *int64            `json:"webhookTimeout"`
	WebhookRetryAttempts *int64            `json:"webhookRetryAttempts"`
	WebhookRetryDelay    *int64            `json:"webhookRetryDelay"`
	ExpectedStatusCodes  *string           `json:"expectedStatusCodes"`
	IsActive             bool              `json:"isActive"`
}

// OwnOrganizationID reads the token's organization id (GET /api/organization).
func (c *Client) OwnOrganizationID(ctx context.Context) (int64, error) {
	var org struct {
		ID int64 `json:"id"`
	}
	return org.ID, c.Do(ctx, http.MethodGet, "/api/organization", nil, &org)
}

func (c *Client) GetEscalationConfig(ctx context.Context, orgID int64) (*EscalationConfig, error) {
	var e EscalationConfig
	return &e, c.Do(ctx, http.MethodGet, "/api/escalation-config/"+strconv.FormatInt(orgID, 10), nil, &e)
}

// UpdateEscalationConfig sends only the keys in patch; the first PATCH stores the configuration.
func (c *Client) UpdateEscalationConfig(ctx context.Context, orgID int64, patch map[string]any) (*EscalationConfig, error) {
	var e EscalationConfig
	return &e, c.Do(ctx, http.MethodPatch, "/api/escalation-config/"+strconv.FormatInt(orgID, 10), patch, &e)
}
