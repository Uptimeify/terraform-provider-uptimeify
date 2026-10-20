package client

import (
	"context"
	"net/http"
	"net/url"
)

// CustomFieldTypes is CUSTOM_FIELD_TYPES (server/lib/custom-field-definitions.ts).
var CustomFieldTypes = []string{"text", "number", "email", "url", "select"}

// CustomField is the field definition of custom-fields.ts.
type CustomField struct {
	ID             int64    `json:"id"`
	OrganizationID int64    `json:"organizationId"`
	Name           string   `json:"name"`
	FieldKey       string   `json:"fieldKey"`
	FieldType      string   `json:"fieldType"`
	IsRequired     bool     `json:"isRequired"`
	DisplayOrder   int64    `json:"displayOrder"`
	Options        []string `json:"options"`
	Placeholder    *string  `json:"placeholder"`
	HelpText       *string  `json:"helpText"`
	ShowInTable    bool     `json:"showInTable"`
	IsActive       bool     `json:"isActive"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

// CreateCustomField posts body; a deactivated field with the same key is reactivated (same id).
func (c *Client) CreateCustomField(ctx context.Context, body map[string]any) (*CustomField, error) {
	var f CustomField
	return &f, c.Do(ctx, http.MethodPost, "/api/custom-fields", body, &f)
}

// GetCustomField also answers deactivated fields (isActive false).
func (c *Client) GetCustomField(ctx context.Context, id string) (*CustomField, error) {
	var f CustomField
	return &f, c.Do(ctx, http.MethodGet, "/api/custom-fields/"+url.PathEscape(id), nil, &f)
}

// ListCustomFields reads the active fields.
func (c *Client) ListCustomFields(ctx context.Context) ([]CustomField, error) {
	var out struct {
		Items []CustomField `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, "/api/custom-fields?envelope=items", nil, &out)
}

func (c *Client) UpdateCustomField(ctx context.Context, id string, patch map[string]any) (*CustomField, error) {
	var f CustomField
	return &f, c.Do(ctx, http.MethodPatch, "/api/custom-fields/"+url.PathEscape(id), patch, &f)
}

// DeleteCustomField only deactivates the field.
func (c *Client) DeleteCustomField(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/custom-fields/"+url.PathEscape(id), nil, nil)
}
