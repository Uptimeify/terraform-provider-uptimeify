package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Status pages, their design, custom domains and the IM rules that drive them
// (server/lib/api-contract/status-pages.ts, im-statuspage-rules.ts).

// StatusPageVisibilities and MonitorKinds mirror the API contract enums.
var (
	StatusPageVisibilities = []string{"public", "customer_members_only", "password_protected"}
	MonitorKinds           = []string{"http", "dns", "icmp", "smtp", "ssh", "tcp", "ftp", "imap_pop"}
	StatusPageTargetStates = []string{"warning", "degraded"}
)

type MonitorRef struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// StatusPage is statusPageView of GET /api/status-pages/{id}.
type StatusPage struct {
	ID                       int64        `json:"id"`
	PublicID                 string       `json:"publicId"`
	OrganizationID           int64        `json:"organizationId"`
	CustomerID               int64        `json:"customerId"`
	CustomerName             string       `json:"customerName"`
	Name                     string       `json:"name"`
	Slug                     string       `json:"slug"`
	Description              *string      `json:"description"`
	Visibility               string       `json:"visibility"`
	IsPublished              bool         `json:"isPublished"`
	SubscriptionsEnabled     bool         `json:"subscriptionsEnabled"`
	SubscriptionDelayMinutes int64        `json:"subscriptionDelayMinutes"`
	HiddenMonitors           []MonitorRef `json:"hiddenMonitors"`
	HasPassword              bool         `json:"hasPassword"`
	AccessPasswordSetAt      *string      `json:"accessPasswordSetAt"`
	CreatedAt                string       `json:"createdAt"`
	UpdatedAt                string       `json:"updatedAt"`
	CustomDomain             *struct {
		ID int64 `json:"id"`
	} `json:"customDomain"`
}

func statusPagePath(id string) string { return "/api/status-pages/" + url.PathEscape(id) }

func (c *Client) GetStatusPage(ctx context.Context, id string) (*StatusPage, error) {
	var p StatusPage
	return &p, c.Do(ctx, http.MethodGet, statusPagePath(id), nil, &p)
}

// ListStatusPages reads GET /api/status-pages?envelope=items (one page with every visible page).
func (c *Client) ListStatusPages(ctx context.Context) ([]StatusPage, error) {
	var env itemsEnvelope[StatusPage]
	return env.Items, c.Do(ctx, http.MethodGet, "/api/status-pages?envelope=items", nil, &env)
}

// CreateStatusPage returns the new page's numeric id and slug (the answer lacks hasPassword and
// customerName, so callers read the page back).
func (c *Client) CreateStatusPage(ctx context.Context, body map[string]any) (string, string, error) {
	var out struct {
		StatusPage struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"statusPage"`
	}
	err := c.Do(ctx, http.MethodPost, "/api/status-pages", body, &out)
	return strconv.FormatInt(out.StatusPage.ID, 10), out.StatusPage.Slug, err
}

func (c *Client) UpdateStatusPage(ctx context.Context, id string, patch map[string]any) error {
	return c.Do(ctx, http.MethodPatch, statusPagePath(id), patch, nil)
}

func (c *Client) DeleteStatusPage(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, statusPagePath(id), nil, nil)
}

// StatusPageDesign is the normalized design of GET/PATCH /api/status-pages/{id}/design.
type StatusPageDesign struct {
	Layout           string `json:"layout"`
	ColorScheme      string `json:"colorScheme"`
	AccentColor      string `json:"accentColor"`
	HeaderStyle      string `json:"headerStyle"`
	FontFamily       string `json:"fontFamily"`
	CardRadius       string `json:"cardRadius"`
	PageWidth        string `json:"pageWidth"`
	CustomTitle      string `json:"customTitle"`
	SEOTitle         string `json:"seoTitle"`
	CustomSubtitle   string `json:"customSubtitle"`
	ShowPoweredBy    bool   `json:"showPoweredBy"`
	ShowUptimeStats  bool   `json:"showUptimeStats"`
	ShowServiceURLs  bool   `json:"showServiceUrls"`
	ShowLastChecked  bool   `json:"showLastChecked"`
	ShowHistory      bool   `json:"showHistory"`
	HistoryDays      int64  `json:"historyDays"`
	ShowIncidents    bool   `json:"showIncidents"`
	IncidentLimit    int64  `json:"incidentLimit"`
	IncidentDays     int64  `json:"incidentDays"`
	ShowMaintenance  bool   `json:"showMaintenance"`
	MaintenanceLimit int64  `json:"maintenanceLimit"`
	MaintenanceDays  int64  `json:"maintenanceDays"`
}

type designEnvelope struct {
	DesignConfig StatusPageDesign `json:"designConfig"`
}

func (c *Client) GetStatusPageDesign(ctx context.Context, pageID string) (*StatusPageDesign, error) {
	var out designEnvelope
	return &out.DesignConfig, c.Do(ctx, http.MethodGet, statusPagePath(pageID)+"/design", nil, &out)
}

// UpdateStatusPageDesign merges patch into the stored design (PATCH .../design).
func (c *Client) UpdateStatusPageDesign(ctx context.Context, pageID string, patch map[string]any) (*StatusPageDesign, error) {
	var out designEnvelope
	return &out.DesignConfig, c.Do(ctx, http.MethodPatch, statusPagePath(pageID)+"/design", patch, &out)
}

// ResetStatusPageDesign stores designConfig null, so the page renders the defaults.
func (c *Client) ResetStatusPageDesign(ctx context.Context, pageID string) error {
	return c.UpdateStatusPage(ctx, pageID, map[string]any{"designConfig": nil})
}

type StatusPageDomain struct {
	ID                int64   `json:"id"`
	Hostname          string  `json:"hostname"`
	Status            string  `json:"status"`
	VerificationToken *string `json:"verificationToken"`
	VerifiedAt        *string `json:"verifiedAt"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

type DomainDNS struct {
	TxtName  string `json:"txtName"`
	TxtValue string `json:"txtValue"`
}

type domainEnvelope struct {
	Domain StatusPageDomain `json:"domain"`
	DNS    DomainDNS        `json:"dns"`
}

func domainPath(id int64) string { return "/api/status-pages/domains/" + strconv.FormatInt(id, 10) }

func (c *Client) CreateStatusPageDomain(ctx context.Context, pageID int64, hostname string) (*StatusPageDomain, *DomainDNS, error) {
	var out domainEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/status-pages/domains", map[string]any{"statusPageId": pageID, "hostname": hostname}, &out)
	return &out.Domain, &out.DNS, err
}

func (c *Client) GetStatusPageDomain(ctx context.Context, id int64) (*StatusPageDomain, *DomainDNS, error) {
	var out domainEnvelope
	err := c.Do(ctx, http.MethodGet, domainPath(id), nil, &out)
	return &out.Domain, &out.DNS, err
}

func (c *Client) DeleteStatusPageDomain(ctx context.Context, id int64) error {
	return c.Do(ctx, http.MethodDelete, domainPath(id), nil, nil)
}

// VerifyStatusPageDomain checks the TXT record; an active domain answers active without a lookup.
func (c *Client) VerifyStatusPageDomain(ctx context.Context, id int64) error {
	return c.Do(ctx, http.MethodPost, "/api/status-pages/domains/verify", map[string]any{"domainId": id}, nil)
}

func (c *Client) ActivateStatusPageDomain(ctx context.Context, id int64) (*StatusPageDomain, error) {
	var out struct {
		Domain StatusPageDomain `json:"domain"`
	}
	return &out.Domain, c.Do(ctx, http.MethodPost, "/api/status-pages/domains/activate", map[string]any{"domainId": id}, &out)
}

// ImStatuspageRule: incidents of a team at minSeverity or worse set a status page to targetState.
type ImStatuspageRule struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organizationId"`
	TeamID         int64  `json:"teamId"`
	StatusPageID   int64  `json:"statusPageId"`
	MinSeverity    string `json:"minSeverity"`
	TargetState    string `json:"targetState"`
	AutoResolve    bool   `json:"autoResolve"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func imRulePath(id string) string { return "/api/im/statuspage-rules/" + url.PathEscape(id) }

func (c *Client) GetImStatuspageRule(ctx context.Context, id string) (*ImStatuspageRule, error) {
	var r ImStatuspageRule
	return &r, c.Do(ctx, http.MethodGet, imRulePath(id), nil, &r)
}

func (c *Client) CreateImStatuspageRule(ctx context.Context, body map[string]any) (*ImStatuspageRule, error) {
	var r ImStatuspageRule
	return &r, c.Do(ctx, http.MethodPost, "/api/im/statuspage-rules", body, &r)
}

func (c *Client) UpdateImStatuspageRule(ctx context.Context, id string, patch map[string]any) (*ImStatuspageRule, error) {
	var r ImStatuspageRule
	return &r, c.Do(ctx, http.MethodPatch, imRulePath(id), patch, &r)
}

func (c *Client) DeleteImStatuspageRule(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, imRulePath(id), nil, nil)
}
