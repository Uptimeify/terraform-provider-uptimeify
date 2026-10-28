package client

import (
	"context"
	"net/http"
)

// Branding is `branding` of GET /api/organization/whitelabel/branding (write fields only).
// The PATCH writes the secondary color as themeNeutral, GET returns it as themeSecondary.
type Branding struct {
	ProductName     *string `json:"productName"`
	HideProductName bool    `json:"hideProductName"`
	HideLogos       bool    `json:"hideLogos"`
	ThemePrimary    *string `json:"themePrimary"`
	ThemeSecondary  *string `json:"themeSecondary"`
	ImprintURL      *string `json:"imprintUrl"`
	PrivacyURL      *string `json:"privacyUrl"`
	TermsURL        *string `json:"termsUrl"`
}

const brandingPath = "/api/organization/whitelabel/branding"

func (c *Client) GetBranding(ctx context.Context) (*Branding, error) {
	var out struct {
		Branding Branding `json:"branding"`
	}
	return &out.Branding, c.Do(ctx, http.MethodGet, brandingPath, nil, &out)
}

// UpdateBranding PATCHes; productName is required in every body (null clears it).
func (c *Client) UpdateBranding(ctx context.Context, body map[string]any) error {
	return c.Do(ctx, http.MethodPatch, brandingPath, body, nil)
}

// UploadBrandingAsset stores one image; kind is logoLight, logoDark, faviconLight or faviconDark,
// dataURL a base64 data: URL. There is no endpoint that removes an uploaded image.
func (c *Client) UploadBrandingAsset(ctx context.Context, kind, fileName, dataURL string) error {
	return c.Do(ctx, http.MethodPost, brandingPath+"/upload",
		map[string]any{"kind": kind, "fileName": fileName, "dataUrl": dataURL}, nil)
}
