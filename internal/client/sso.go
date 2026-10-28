package client

import (
	"context"
	"net/http"
)

// SSO is GET/PUT /api/organization/sso (server/lib/auth/sso-admin.ts buildSsoView). Without a
// configuration GET answers only configured=false, redirectUri and scopes. The client secret never
// comes back, only hasClientSecret.
type SSO struct {
	Configured      bool     `json:"configured"`
	Type            string   `json:"type"`
	Domain          string   `json:"domain"`
	DomainVerified  bool     `json:"domainVerified"`
	TxtRecordName   string   `json:"txtRecordName"`
	TxtRecordValue  string   `json:"txtRecordValue"`
	DefaultRole     string   `json:"defaultRole"`
	EnforceSSO      bool     `json:"enforceSso"`
	CanEnforce      bool     `json:"canEnforce"`
	Issuer          string   `json:"issuer"`
	ClientID        string   `json:"clientId"`
	HasClientSecret bool     `json:"hasClientSecret"`
	RedirectURI     string   `json:"redirectUri"`
	Scopes          []string `json:"scopes"`
}

// SSODomainCheck is the answer of POST /api/organization/sso/verify-domain.
type SSODomainCheck struct {
	Verified bool     `json:"verified"`
	Expected string   `json:"expected"`
	Found    []string `json:"found"`
}

const ssoPath = "/api/organization/sso"

func (c *Client) GetSSO(ctx context.Context) (*SSO, error) {
	var s SSO
	return &s, c.Do(ctx, http.MethodGet, ssoPath, nil, &s)
}

// PutSSO replaces the configuration; a body without clientSecret keeps the stored one.
func (c *Client) PutSSO(ctx context.Context, body map[string]any) (*SSO, error) {
	var s SSO
	return &s, c.Do(ctx, http.MethodPut, ssoPath, body, &s)
}

func (c *Client) DeleteSSO(ctx context.Context) error {
	return c.Do(ctx, http.MethodDelete, ssoPath, nil, nil)
}

// VerifySSODomain looks up the TXT record once; verified=false is a normal answer, not an error.
func (c *Client) VerifySSODomain(ctx context.Context) (*SSODomainCheck, error) {
	var r SSODomainCheck
	return &r, c.Do(ctx, http.MethodPost, ssoPath+"/verify-domain", nil, &r)
}
