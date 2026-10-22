package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Opt is a response field the API may leave out: Set reports whether the key was present
// (a JSON null included), V holds the value.
type Opt[T any] struct {
	Set bool
	V   *T
}

func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

// CustomerDetail is GET /api/customers/{id} and the POST answer (customers.ts customerRow).
type CustomerDetail struct {
	ID                      int64             `json:"id"`
	PublicID                string            `json:"publicId"`
	OrganizationID          int64             `json:"organizationId"`
	Name                    string            `json:"name"`
	Email                   *string           `json:"email"`
	NotificationPhoneNumber *string           `json:"notificationPhoneNumber"`
	NotificationEmail       *string           `json:"notificationEmail"`
	CompanyName             *string           `json:"companyName"`
	PackageID               *int64            `json:"packageId"`
	PackageType             string            `json:"packageType"`
	PackageDisplayName      *string           `json:"packageDisplayName"`
	Status                  string            `json:"status"`
	CustomFields            json.RawMessage   `json:"customFields"`
	MonthlyReportsEnabled   bool              `json:"monthlyReportsEnabled"`
	EmailLanguage           *string           `json:"emailLanguage"`
	NotificationTargets     map[string]string `json:"notificationTargets"`
	CreatedAt               string            `json:"createdAt"`
	UpdatedAt               string            `json:"updatedAt"`

	// Permission overrides (null = package default). GET leaves them out for callers that may not
	// write them (lib/customers/permission-overrides.ts), so absent means "unknown", not null.
	AllowSelfService            Opt[bool]  `json:"allowSelfService"`
	MaxSelfServiceURLs          Opt[int64] `json:"maxSelfServiceUrls"`
	CanEditManaged              Opt[bool]  `json:"canEditManaged"`
	EnableEmailAlerts           Opt[bool]  `json:"enableEmailAlerts"`
	EnableSmsAlerts             Opt[bool]  `json:"enableSmsAlerts"`
	EnableWebhookAlerts         Opt[bool]  `json:"enableWebhookAlerts"`
	EnableIntegrationAlerts     Opt[bool]  `json:"enableIntegrationAlerts"`
	EnablePostRequestEscalation Opt[bool]  `json:"enablePostRequestEscalation"`
}

func (c *Client) GetCustomerDetail(ctx context.Context, id string) (*CustomerDetail, error) {
	var cu CustomerDetail
	return &cu, c.Do(ctx, http.MethodGet, "/api/customers/"+url.PathEscape(id), nil, &cu)
}

// CreateCustomer posts body (customerCreateSchema keys) and returns the written row.
func (c *Client) CreateCustomer(ctx context.Context, body map[string]any) (*CustomerDetail, error) {
	var out struct {
		Customer CustomerDetail `json:"customer"`
	}
	if err := c.Do(ctx, http.MethodPost, "/api/customers", body, &out); err != nil {
		return nil, err
	}
	return &out.Customer, nil
}

// UpdateCustomer sends only the keys in patch (customerPatchSchema).
func (c *Client) UpdateCustomer(ctx context.Context, id string, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, "/api/customers/"+url.PathEscape(id), patch, nil)
}

// DeleteCustomer hard-deletes the customer; userAction (deactivate|delete) decides about logins
// assigned only to it.
func (c *Client) DeleteCustomer(ctx context.Context, id, userAction string) error {
	p := "/api/customers/" + url.PathEscape(id)
	if userAction != "" {
		p += "?userAction=" + url.QueryEscape(userAction)
	}
	return c.Do(ctx, http.MethodDelete, p, nil, nil)
}

// CustomerIP is the ipRow of customers.ts (DNSBL monitoring of one address).
type CustomerIP struct {
	ID             int64   `json:"id"`
	PublicID       string  `json:"publicId"`
	CustomerID     int64   `json:"customerId"`
	Label          *string `json:"label"`
	IPAddress      string  `json:"ipAddress"`
	IPFamily       string  `json:"ipFamily"`
	Status         string  `json:"status"`
	ManagementType string  `json:"managementType"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	Deduplicated   bool    `json:"deduplicated"`
}

func (c *Client) CreateCustomerIP(ctx context.Context, customerID string, body map[string]any) (*CustomerIP, error) {
	var ip CustomerIP
	return &ip, c.Do(ctx, http.MethodPost, "/api/customers/"+url.PathEscape(customerID)+"/ips", body, &ip)
}

func (c *Client) GetCustomerIP(ctx context.Context, id string) (*CustomerIP, error) {
	var ip CustomerIP
	return &ip, c.Do(ctx, http.MethodGet, "/api/customer-ips/"+url.PathEscape(id), nil, &ip)
}

func (c *Client) UpdateCustomerIP(ctx context.Context, id string, patch map[string]any) (*CustomerIP, error) {
	var ip CustomerIP
	return &ip, c.Do(ctx, http.MethodPatch, "/api/customer-ips/"+url.PathEscape(id), patch, &ip)
}

func (c *Client) DeleteCustomerIP(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/customer-ips/"+url.PathEscape(id), nil, nil)
}

// CustomerDomain is the domainRow of customers.ts (domain expiry monitoring over RDAP).
type CustomerDomain struct {
	ID                int64   `json:"id"`
	PublicID          string  `json:"publicId"`
	CustomerID        int64   `json:"customerId"`
	Label             *string `json:"label"`
	DomainName        string  `json:"domainName"`
	Status            string  `json:"status"`
	ManagementType    string  `json:"managementType"`
	ExpiryWarningDays int64   `json:"expiryWarningDays"`
	ExpiryErrorDays   int64   `json:"expiryErrorDays"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	Deduplicated      bool    `json:"deduplicated"`
}

// CreateCustomerDomain needs the customer's public id in the path (the handler refuses numeric ids).
func (c *Client) CreateCustomerDomain(ctx context.Context, customerPublicID string, body map[string]any) (*CustomerDomain, error) {
	var d CustomerDomain
	return &d, c.Do(ctx, http.MethodPost, "/api/customers/"+url.PathEscape(customerPublicID)+"/domains", body, &d)
}

func (c *Client) GetCustomerDomain(ctx context.Context, id string) (*CustomerDomain, error) {
	var d CustomerDomain
	return &d, c.Do(ctx, http.MethodGet, "/api/customer-domains/"+url.PathEscape(id), nil, &d)
}

func (c *Client) UpdateCustomerDomain(ctx context.Context, id string, patch map[string]any) (*CustomerDomain, error) {
	var d CustomerDomain
	return &d, c.Do(ctx, http.MethodPatch, "/api/customer-domains/"+url.PathEscape(id), patch, &d)
}

func (c *Client) DeleteCustomerDomain(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/customer-domains/"+url.PathEscape(id), nil, nil)
}
