package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Website is the GET /api/websites/{id} row of the API contract
// (server/lib/api-contract/websites.ts, websiteRead). Secrets (custom headers, request body,
// authorization header, basic auth password, mTLS) never come back; only hasCustomHeaders and
// hasRequestBody say whether headers or a body are stored.
type Website struct {
	ID                           int64    `json:"id"`
	PublicID                     string   `json:"publicId"`
	CustomerID                   int64    `json:"customerId"`
	Name                         string   `json:"name"`
	URL                          string   `json:"url"`
	MonitoringType               string   `json:"monitoringType"`
	Status                       string   `json:"status"`
	CheckInterval                int64    `json:"checkInterval"`
	TimeoutSeconds               int64    `json:"timeoutSeconds"`
	ExpectedStatusCodes          *string  `json:"expectedStatusCodes"`
	SearchTerm                   *string  `json:"searchTerm"`
	AllowedCheckCountryCodes     []string `json:"allowedCheckCountryCodes"`
	AuthMode                     string   `json:"authMode"`
	BasicAuthUsername            *string  `json:"basicAuthUsername"`
	HTTPMethod                   *string  `json:"httpMethod"`
	FollowRedirects              *bool    `json:"followRedirects"`
	CookieHandling               *string  `json:"cookieHandling"`
	ConnectHost                  *string  `json:"connectHost"`
	ConnectPort                  *int64   `json:"connectPort"`
	ConnectTLSInsecure           bool     `json:"connectTlsInsecure"`
	CheckSSLEnabled              *bool    `json:"checkSslEnabled"`
	CheckHTTPSRedirectEnabled    *bool    `json:"checkHttpsRedirectEnabled"`
	CheckStatusEnabled           *bool    `json:"checkStatusEnabled"`
	CheckSizeEnabled             *bool    `json:"checkSizeEnabled"`
	CheckResponseTimeEnabled     *bool    `json:"checkResponseTimeEnabled"`
	CheckKeywordEnabled          *bool    `json:"checkKeywordEnabled"`
	CheckDomainExpiryEnabled     *bool    `json:"checkDomainExpiryEnabled"`
	CheckExpectedResponseEnabled *bool    `json:"checkExpectedResponseEnabled"`
	ExpectedResponseMatchType    *string  `json:"expectedResponseMatchType"`
	ExpectedResponseValue        *string  `json:"expectedResponseValue"`
	ExpectedResponseJSONPath     *string  `json:"expectedResponseJsonPath"`
	SSLNoticeDays                *int64   `json:"sslNoticeDays"`
	SSLErrorDays                 *int64   `json:"sslErrorDays"`
	DomainExpiryNoticeDays       *int64   `json:"domainExpiryNoticeDays"`
	DomainExpiryErrorDays        *int64   `json:"domainExpiryErrorDays"`
	MinPageSize                  *int64   `json:"minPageSize"`
	MaxPageSize                  *int64   `json:"maxPageSize"`
	HeartbeatToken               *string  `json:"heartbeatToken"`
	HeartbeatGracePeriodMinutes  *int64   `json:"heartbeatGracePeriodMinutes"`
	HasCustomHeaders             bool     `json:"hasCustomHeaders"`
	HasRequestBody               bool     `json:"hasRequestBody"`
	CreatedAt                    string   `json:"createdAt"`
	UpdatedAt                    string   `json:"updatedAt"`
}

func (c *Client) GetWebsite(ctx context.Context, id string) (*Website, error) {
	var w Website
	return &w, c.Do(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(id), nil, &w)
}

// CreateWebsite posts body (createWebsiteSchema keys) and returns the new numeric id.
// The POST answer lacks hasCustomHeaders/hasRequestBody, so callers read the row back with GetWebsite.
func (c *Client) CreateWebsite(ctx context.Context, body map[string]any) (string, error) {
	var w Website
	if err := c.Do(ctx, http.MethodPost, "/api/websites", body, &w); err != nil {
		return "", err
	}
	return strconv.FormatInt(w.ID, 10), nil
}

// UpdateWebsite sends PATCH /api/websites/{id}. The handler has two modes: a body carrying
// customerId, name and url together is a full update (createWebsiteSchema keys), anything else a
// partial one. The caller decides which by the keys it puts in body.
func (c *Client) UpdateWebsite(ctx context.Context, id string, body map[string]any) error {
	return c.Do(ctx, http.MethodPatch, "/api/websites/"+url.PathEscape(id), body, nil)
}

func (c *Client) DeleteWebsite(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/websites/"+url.PathEscape(id), nil, nil)
}

// RotateHeartbeatToken mints a new heartbeat token; the old one stops working at once. The server
// also rewrites url to heartbeat://<token>. 400 heartbeatMonitorRequired for other types.
func (c *Client) RotateHeartbeatToken(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, "/api/websites/"+url.PathEscape(id)+"/heartbeat-token/rotate", nil, nil)
}
