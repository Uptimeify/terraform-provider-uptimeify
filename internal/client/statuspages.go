package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Status pages, their design, custom domains and the IM rules that drive them
// (server/lib/api-contract/status-pages.ts, im-statuspage-rules.ts).

// StatusPageVisibilities and MonitorKinds mirror the API contract enums.
var (
	StatusPageVisibilities = []string{"public", "customer_members_only", "password_protected", "sso_members_only"}
	MonitorKinds           = []string{"http", "dns", "icmp", "smtp", "ssh", "tcp", "ftp", "imap_pop"}
	StatusPageTargetStates = []string{"warning", "degraded"}
	// ComponentSourceKinds and ComponentManualStatuses mirror createStatusPageComponentSchema.
	ComponentSourceKinds    = []string{"monitor", "manual", "dependency"}
	ComponentManualStatuses = []string{"operational", "degraded", "partial_outage", "major_outage", "maintenance"}
)

type MonitorRef struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// AudienceGroup is one entry of audienceGroups (0328): components only visitors with a verified
// email of one of the domains see. componentIds come back numeric.
type AudienceGroup struct {
	Name         string   `json:"name"`
	EmailDomains []string `json:"emailDomains"`
	ComponentIDs []int64  `json:"componentIds"`
}

// StatusPage is statusPageView of GET /api/status-pages/{id}.
type StatusPage struct {
	ID                       int64           `json:"id"`
	PublicID                 string          `json:"publicId"`
	OrganizationID           int64           `json:"organizationId"`
	CustomerID               *int64          `json:"customerId"` // null: page of the whole organization
	CustomerName             *string         `json:"customerName"`
	Name                     string          `json:"name"`
	Slug                     string          `json:"slug"`
	Description              *string         `json:"description"`
	Visibility               string          `json:"visibility"`
	IsPublished              bool            `json:"isPublished"`
	SubscriptionsEnabled     bool            `json:"subscriptionsEnabled"`
	SubscriptionDelayMinutes int64           `json:"subscriptionDelayMinutes"`
	IncludeCustomerMonitors  bool            `json:"includeCustomerMonitors"`
	HiddenMonitors           []MonitorRef    `json:"hiddenMonitors"`
	SMSSubscriptionsEnabled  bool            `json:"smsSubscriptionsEnabled"`
	PushSubscriptionsEnabled bool            `json:"pushSubscriptionsEnabled"`
	IPAllowlist              []string        `json:"ipAllowlist"`
	AudienceGroups           []AudienceGroup `json:"audienceGroups"`
	HasPassword              bool            `json:"hasPassword"`
	AccessPasswordSetAt      *string         `json:"accessPasswordSetAt"`
	CreatedAt                string          `json:"createdAt"`
	UpdatedAt                string          `json:"updatedAt"`
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
	CustomCSS        string `json:"customCss"`
	// StatusLabels: own texts per state (operational, degraded, partialOutage, majorOutage,
	// maintenance); a state without own text is absent.
	StatusLabels map[string]string `json:"statusLabels"`
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
	StatusPageID      *int64  `json:"statusPageId"` // null while unbound
	Hostname          string  `json:"hostname"`
	Status            string  `json:"status"`
	VerificationToken *string `json:"verificationToken"`
	VerifiedAt        *string `json:"verifiedAt"`
	// Diagnosis of the last failed TXT verification, null after a successful one.
	LastVerificationError   json.RawMessage `json:"lastVerificationError"`
	LastVerificationErrorAt *string         `json:"lastVerificationErrorAt"`
	CreatedAt               string          `json:"createdAt"`
	UpdatedAt               string          `json:"updatedAt"`
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

// StatusPageComponent is one row of GET /api/status-pages/{id}/components.
type StatusPageComponent struct {
	ID             int64   `json:"id"`
	PublicID       string  `json:"publicId"`
	StatusPageID   int64   `json:"statusPageId"`
	GroupID        *int64  `json:"groupId"`
	Sort           int64   `json:"sort"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	ShowUptime     bool    `json:"showUptime"`
	SourceKind     string  `json:"sourceKind"`
	MonitorType    *string `json:"monitorType"`
	MonitorID      *int64  `json:"monitorId"`
	ManualStatus   *string `json:"manualStatus"`
	DependencySlug *string `json:"dependencySlug"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// StatusPageComponentGroup is one row of GET /api/status-pages/{id}/groups.
type StatusPageComponentGroup struct {
	ID           int64  `json:"id"`
	PublicID     string `json:"publicId"`
	StatusPageID int64  `json:"statusPageId"`
	Name         string `json:"name"`
	Sort         int64  `json:"sort"`
	Collapsed    bool   `json:"collapsed"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func pageSubPath(pageID int64, kind string) string {
	return statusPagePath(strconv.FormatInt(pageID, 10)) + "/" + kind
}

func pageSubItemPath(pageID int64, kind string, id int64) string {
	return pageSubPath(pageID, kind) + "/" + strconv.FormatInt(id, 10)
}

// There is no single GET; the lists hold every row of a page (at most 100 components).
func findByID[T any](items []T, id func(T) int64, want int64) (*T, error) {
	for i := range items {
		if id(items[i]) == want {
			return &items[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, StatusMessage: "not found"}
}

func (c *Client) GetStatusPageComponent(ctx context.Context, pageID, id int64) (*StatusPageComponent, error) {
	var env itemsEnvelope[StatusPageComponent]
	if err := c.Do(ctx, http.MethodGet, pageSubPath(pageID, "components"), nil, &env); err != nil {
		return nil, err
	}
	return findByID(env.Items, func(x StatusPageComponent) int64 { return x.ID }, id)
}

func (c *Client) CreateStatusPageComponent(ctx context.Context, pageID int64, body map[string]any) (*StatusPageComponent, error) {
	var out StatusPageComponent
	return &out, c.Do(ctx, http.MethodPost, pageSubPath(pageID, "components"), body, &out)
}

func (c *Client) UpdateStatusPageComponent(ctx context.Context, pageID, id int64, patch map[string]any) (*StatusPageComponent, error) {
	var out StatusPageComponent
	return &out, c.Do(ctx, http.MethodPatch, pageSubItemPath(pageID, "components", id), patch, &out)
}

func (c *Client) DeleteStatusPageComponent(ctx context.Context, pageID, id int64) error {
	return c.Do(ctx, http.MethodDelete, pageSubItemPath(pageID, "components", id), nil, nil)
}

func (c *Client) GetStatusPageComponentGroup(ctx context.Context, pageID, id int64) (*StatusPageComponentGroup, error) {
	var env itemsEnvelope[StatusPageComponentGroup]
	if err := c.Do(ctx, http.MethodGet, pageSubPath(pageID, "groups"), nil, &env); err != nil {
		return nil, err
	}
	return findByID(env.Items, func(x StatusPageComponentGroup) int64 { return x.ID }, id)
}

func (c *Client) CreateStatusPageComponentGroup(ctx context.Context, pageID int64, body map[string]any) (*StatusPageComponentGroup, error) {
	var out StatusPageComponentGroup
	return &out, c.Do(ctx, http.MethodPost, pageSubPath(pageID, "groups"), body, &out)
}

func (c *Client) UpdateStatusPageComponentGroup(ctx context.Context, pageID, id int64, patch map[string]any) (*StatusPageComponentGroup, error) {
	var out StatusPageComponentGroup
	return &out, c.Do(ctx, http.MethodPatch, pageSubItemPath(pageID, "groups", id), patch, &out)
}

func (c *Client) DeleteStatusPageComponentGroup(ctx context.Context, pageID, id int64) error {
	return c.Do(ctx, http.MethodDelete, pageSubItemPath(pageID, "groups", id), nil, nil)
}

// StatusPageMaintenance is GET /api/status-pages/maintenances/{id}: a planned maintenance
// announced on status pages (not a monitor maintenance window).
type StatusPageMaintenance struct {
	ID                    int64   `json:"id"`
	PublicID              string  `json:"publicId"`
	Title                 string  `json:"title"`
	Description           *string `json:"description"`
	ScheduledFor          string  `json:"scheduledFor"`
	ScheduledUntil        string  `json:"scheduledUntil"`
	State                 string  `json:"state"`
	NotifyOnSchedule      bool    `json:"notifyOnSchedule"`
	ReminderMinutesBefore *int64  `json:"reminderMinutesBefore"`
	NotifyOnStart         bool    `json:"notifyOnStart"`
	NotifyOnComplete      bool    `json:"notifyOnComplete"`
	CancelledAt           *string `json:"cancelledAt"`
	StatusPageIDs         []int64 `json:"statusPageIds"`
	ComponentIDs          []int64 `json:"componentIds"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
}

const statusPageMaintenancesPath = "/api/status-pages/maintenances"

func statusPageMaintenancePath(id string) string {
	return statusPageMaintenancesPath + "/" + url.PathEscape(id)
}

// GetStatusPageMaintenance takes the numeric id or the public id.
func (c *Client) GetStatusPageMaintenance(ctx context.Context, id string) (*StatusPageMaintenance, error) {
	var out StatusPageMaintenance
	return &out, c.Do(ctx, http.MethodGet, statusPageMaintenancePath(id), nil, &out)
}

func (c *Client) CreateStatusPageMaintenance(ctx context.Context, body map[string]any) (*StatusPageMaintenance, error) {
	var out StatusPageMaintenance
	return &out, c.Do(ctx, http.MethodPost, statusPageMaintenancesPath, body, &out)
}

func (c *Client) UpdateStatusPageMaintenance(ctx context.Context, id string, patch map[string]any) (*StatusPageMaintenance, error) {
	var out StatusPageMaintenance
	return &out, c.Do(ctx, http.MethodPatch, statusPageMaintenancePath(id), patch, &out)
}

func (c *Client) DeleteStatusPageMaintenance(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, statusPageMaintenancePath(id), nil, nil)
}

// CancelStatusPageMaintenance cancels an upcoming maintenance (tests and drift only; the
// resource does not model lifecycle actions).
func (c *Client) CancelStatusPageMaintenance(ctx context.Context, id string) (*StatusPageMaintenance, error) {
	var out StatusPageMaintenance
	return &out, c.Do(ctx, http.MethodPost, statusPageMaintenancePath(id)+"/cancel", map[string]any{}, &out)
}
