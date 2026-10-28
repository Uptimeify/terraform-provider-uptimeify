package client

import (
	"context"
	"net/http"
	"net/url"
)

// NotificationStep is one step of a personal notification chain (app migration 0325).
type NotificationStep struct {
	Urgency      string `json:"urgency"`
	DelayMinutes int64  `json:"delayMinutes"`
	Channel      string `json:"channel"`
}

// UserNotificationSettings is GET/PUT /api/im/users/{userId}/notification-settings.
type UserNotificationSettings struct {
	UserID        string             `json:"userId"`
	PhoneNumber   *string            `json:"phoneNumber"`
	PhoneVerified bool               `json:"phoneVerified"`
	Channels      map[string]bool    `json:"channels"`
	Rules         []NotificationStep `json:"rules"`
}

func (c *Client) GetUserNotificationSettings(ctx context.Context, userID string) (*UserNotificationSettings, error) {
	var s UserNotificationSettings
	return &s, c.Do(ctx, http.MethodGet, "/api/im/users/"+url.PathEscape(userID)+"/notification-settings", nil, &s)
}

// PutUserNotificationSettings replaces the user's whole chain; body keys: rules, channels, phoneNumber.
func (c *Client) PutUserNotificationSettings(ctx context.Context, userID string, body map[string]any) (*UserNotificationSettings, error) {
	var s UserNotificationSettings
	return &s, c.Do(ctx, http.MethodPut, "/api/im/users/"+url.PathEscape(userID)+"/notification-settings", body, &s)
}

// NotificationTemplate is GET/PUT /api/im/notification-template.
type NotificationTemplate struct {
	Channels  map[string]bool    `json:"channels"`
	Rules     []NotificationStep `json:"rules"`
	UpdatedAt *string            `json:"updatedAt"`
}

func (c *Client) GetNotificationTemplate(ctx context.Context) (*NotificationTemplate, error) {
	var t NotificationTemplate
	return &t, c.Do(ctx, http.MethodGet, "/api/im/notification-template", nil, &t)
}

func (c *Client) PutNotificationTemplate(ctx context.Context, body map[string]any) (*NotificationTemplate, error) {
	var t NotificationTemplate
	return &t, c.Do(ctx, http.MethodPut, "/api/im/notification-template", body, &t)
}
