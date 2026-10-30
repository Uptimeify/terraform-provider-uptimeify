package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Organization logins, settings and tokens (server/lib/api-contract/users.ts, organizations.ts,
// organization-settings.ts, customers.ts for the customer tokens).

// UserCustomerAccess is one entry of user.customerAccess.
type UserCustomerAccess struct {
	CustomerID   int64  `json:"customerId"`
	CustomerName string `json:"customerName"`
}

// UserDetail is GET /api/users/{id}.
type UserDetail struct {
	ID             string               `json:"id"`
	Email          string               `json:"email"`
	Name           *string              `json:"name"`
	FirstName      *string              `json:"firstName"`
	LastName       *string              `json:"lastName"`
	Role           string               `json:"role"`
	IsActive       bool                 `json:"isActive"`
	CreatedAt      string               `json:"createdAt"`
	CustomerAccess []UserCustomerAccess `json:"customerAccess"`
	Language       *string              `json:"language"` // null = organization language
}

// UserCreate is the POST /api/users body (an invitation; the login sets its own password).
type UserCreate struct {
	Email       string  `json:"email"`
	FirstName   *string `json:"firstName,omitempty"`
	LastName    *string `json:"lastName,omitempty"`
	Role        string  `json:"role"`
	CustomerIDs []int64 `json:"customerIds,omitempty"`
	Language    *string `json:"language,omitempty"`
}

// CreateUser invites a login and returns its id.
func (c *Client) CreateUser(ctx context.Context, in UserCreate) (string, error) {
	var out struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	return out.User.ID, c.Do(ctx, http.MethodPost, "/api/users", in, &out)
}

func (c *Client) GetUser(ctx context.Context, id string) (*UserDetail, error) {
	var u UserDetail
	return &u, c.Do(ctx, http.MethodGet, "/api/users/"+url.PathEscape(id), nil, &u)
}

func (c *Client) UpdateUser(ctx context.Context, id string, patch map[string]any) (*UserDetail, error) {
	var out struct {
		User UserDetail `json:"user"`
	}
	return &out.User, c.Do(ctx, http.MethodPatch, "/api/users/"+url.PathEscape(id), patch, &out)
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/api/users/"+url.PathEscape(id), nil, nil)
}

// NotificationChannels / NotificationTargets are the organization defaults for new customers.
type NotificationChannels struct {
	Email        bool `json:"email" tfsdk:"email"`
	SMS          bool `json:"sms" tfsdk:"sms"`
	Webhook      bool `json:"webhook" tfsdk:"webhook"`
	Integrations bool `json:"integrations" tfsdk:"integrations"`
}

type NotificationTargets struct {
	Email        string `json:"email" tfsdk:"email"`
	SMS          string `json:"sms" tfsdk:"sms"`
	Webhook      string `json:"webhook" tfsdk:"webhook"`
	Integrations string `json:"integrations" tfsdk:"integrations"`
}

// Organization is GET /api/organization. The tfsdk tags let the uptimeify_organization data
// source store it as is; admin-only fields are absent (nil) for other roles.
type Organization struct {
	ID                              int64                 `json:"id" tfsdk:"id"`
	PublicID                        string                `json:"publicId" tfsdk:"public_id"`
	Name                            string                `json:"name" tfsdk:"name"`
	Status                          string                `json:"status" tfsdk:"status"`
	DeletionDueAt                   *string               `json:"deletionDueAt" tfsdk:"deletion_due_at"`
	EmailLanguage                   *string               `json:"emailLanguage" tfsdk:"email_language"`
	BillingEmail                    *string               `json:"billingEmail" tfsdk:"billing_email"`
	BillingCycle                    *string               `json:"billingCycle" tfsdk:"billing_cycle"`
	PaymentMethod                   *string               `json:"paymentMethod" tfsdk:"payment_method"`
	DefaultNotificationChannels     *NotificationChannels `json:"defaultNotificationChannels" tfsdk:"default_notification_channels"`
	DefaultNotificationTargets      *NotificationTargets  `json:"defaultNotificationTargets" tfsdk:"default_notification_targets"`
	RequireMfaAdmins                *bool                 `json:"requireMfaAdmins" tfsdk:"require_mfa_admins"`
	RequireMfaEditors               *bool                 `json:"requireMfaEditors" tfsdk:"require_mfa_editors"`
	RequireMfaReadonly              *bool                 `json:"requireMfaReadonly" tfsdk:"require_mfa_readonly"`
	RequireMfaCustomers             *bool                 `json:"requireMfaCustomers" tfsdk:"require_mfa_customers"`
	CompanyName                     *string               `json:"companyName" tfsdk:"company_name"`
	Street                          *string               `json:"street" tfsdk:"street"`
	PostalCode                      *string               `json:"postalCode" tfsdk:"postal_code"`
	City                            *string               `json:"city" tfsdk:"city"`
	Country                         *string               `json:"country" tfsdk:"country"`
	VatID                           *string               `json:"vatId" tfsdk:"vat_id"`
	QuotaWebsitesLimit              *int64                `json:"quotaWebsitesLimit" tfsdk:"quota_websites_limit"`
	QuotaIpsLimit                   *int64                `json:"quotaIpsLimit" tfsdk:"quota_ips_limit"`
	QuotaSmsIncluded                *int64                `json:"quotaSmsIncluded" tfsdk:"quota_sms_included"`
	QuotaMonthlyPriceCents          *int64                `json:"quotaMonthlyPriceCents" tfsdk:"quota_monthly_price_cents"`
	QuotaExtraSmsPriceCents         *int64                `json:"quotaExtraSmsPriceCents" tfsdk:"quota_extra_sms_price_cents"`
	QuotaAutoUpgradeEnabled         *bool                 `json:"quotaAutoUpgradeEnabled" tfsdk:"quota_auto_upgrade_enabled"`
	SmsOverageSafetyCapCentsMonthly *int64                `json:"smsOverageSafetyCapCentsMonthly" tfsdk:"sms_overage_safety_cap_cents_monthly"`
}

func (c *Client) GetOrganization(ctx context.Context) (*Organization, error) {
	var o Organization
	return &o, c.Do(ctx, http.MethodGet, "/api/organization", nil, &o)
}

// UpdateOrganization PATCHes the token's own organization. The server drops guarded fields
// (MFA switches, SMS cap) whose value does not change.
func (c *Client) UpdateOrganization(ctx context.Context, patch map[string]any) (*Organization, error) {
	var o Organization
	return &o, c.Do(ctx, http.MethodPatch, "/api/organization", patch, &o)
}

// PackageConfig is one row of GET /api/package-configs (catalog, bare array), stored as is by
// the uptimeify_package_configs data source.
type PackageConfig struct {
	ID                          int64    `json:"id" tfsdk:"id"`
	PackageType                 string   `json:"packageType" tfsdk:"package_type"`
	DisplayName                 *string  `json:"displayName" tfsdk:"display_name"`
	MaxURLs                     *int64   `json:"maxUrls" tfsdk:"max_urls"`
	MaxStatusPages              *int64   `json:"maxStatusPages" tfsdk:"max_status_pages"` // null = unlimited
	AllowedCheckCountryCodes    []string `json:"allowedCheckCountryCodes" tfsdk:"allowed_check_country_codes"`
	DataRetentionMonths         *int64   `json:"dataRetentionMonths" tfsdk:"data_retention_months"`
	CheckIntervalMinutes        *int64   `json:"checkIntervalMinutes" tfsdk:"check_interval_minutes"`
	CheckLocations              *int64   `json:"checkLocations" tfsdk:"check_locations"`
	NotificationDelayMinutes    *int64   `json:"notificationDelayMinutes" tfsdk:"notification_delay_minutes"`
	ReminderDelayMinutes        *int64   `json:"reminderDelayMinutes" tfsdk:"reminder_delay_minutes"`
	AlertConsecutiveChecks      *int64   `json:"alertConsecutiveChecks" tfsdk:"alert_consecutive_checks"`
	AlertLocationThreshold      *string  `json:"alertLocationThreshold" tfsdk:"alert_location_threshold"`
	AlertLocationThresholdCount *int64   `json:"alertLocationThresholdCount" tfsdk:"alert_location_threshold_count"`
	AlertReminderInterval       *int64   `json:"alertReminderInterval" tfsdk:"alert_reminder_interval"`
	DnsblReminderInterval       *int64   `json:"dnsblReminderInterval" tfsdk:"dnsbl_reminder_interval"`
	SslExpiryReminderInterval   *int64   `json:"sslExpiryReminderInterval" tfsdk:"ssl_expiry_reminder_interval"`
	EnableSslCheck              bool     `json:"enableSslCheck" tfsdk:"enable_ssl_check"`
	EnableHTTPSCheck            bool     `json:"enableHttpsCheck" tfsdk:"enable_https_check"`
	EnableStatusCheck           bool     `json:"enableStatusCheck" tfsdk:"enable_status_check"`
	EnableSizeCheck             bool     `json:"enableSizeCheck" tfsdk:"enable_size_check"`
	EnableResponseTimeCheck     bool     `json:"enableResponseTimeCheck" tfsdk:"enable_response_time_check"`
	EnableKeywordCheck          bool     `json:"enableKeywordCheck" tfsdk:"enable_keyword_check"`
	EnableEmailAlerts           bool     `json:"enableEmailAlerts" tfsdk:"enable_email_alerts"`
	EnableSmsAlerts             bool     `json:"enableSmsAlerts" tfsdk:"enable_sms_alerts"`
	EnableWebhookAlerts         bool     `json:"enableWebhookAlerts" tfsdk:"enable_webhook_alerts"`
	EnableIntegrationAlerts     bool     `json:"enableIntegrationAlerts" tfsdk:"enable_integration_alerts"`
	EnablePostRequestEscalation bool     `json:"enablePostRequestEscalation" tfsdk:"enable_post_request_escalation"`
	EnableMaintenanceWindows    bool     `json:"enableMaintenanceWindows" tfsdk:"enable_maintenance_windows"`
	EnablePdfReports            bool     `json:"enablePdfReports" tfsdk:"enable_pdf_reports"`
	MonthlyReportsDefault       bool     `json:"monthlyReportsDefault" tfsdk:"monthly_reports_default"`
	AllowSelfService            bool     `json:"allowSelfService" tfsdk:"allow_self_service"`
	MaxSelfServiceURLs          int64    `json:"maxSelfServiceUrls" tfsdk:"max_self_service_urls"`
	FlappingEnabled             bool     `json:"flappingEnabled" tfsdk:"flapping_enabled"`
	FlappingIncidentCount       int64    `json:"flappingIncidentCount" tfsdk:"flapping_incident_count"`
	FlappingWindowMinutes       int64    `json:"flappingWindowMinutes" tfsdk:"flapping_window_minutes"`
	FlappingRecoveryCleanCycles int64    `json:"flappingRecoveryCleanCycles" tfsdk:"flapping_recovery_clean_cycles"`
	AlertRepeatIntervalMinutes  int64    `json:"alertRepeatIntervalMinutes" tfsdk:"alert_repeat_interval_minutes"`
	Notes                       *string  `json:"notes" tfsdk:"notes"`
	UsedByCustomerCount         int64    `json:"usedByCustomerCount" tfsdk:"used_by_customer_count"`
}

func (c *Client) ListPackageConfigs(ctx context.Context) ([]PackageConfig, error) {
	var out []PackageConfig
	return out, c.Do(ctx, http.MethodGet, "/api/package-configs", nil, &out)
}

func packageConfigPath(packageType string) string {
	return "/api/package-configs/" + url.PathEscape(packageType)
}

// UpsertPackageConfig PATCHes one package by type: creates it if missing, else merges the patch.
func (c *Client) UpsertPackageConfig(ctx context.Context, packageType string, patch map[string]any) (*PackageConfig, error) {
	var out PackageConfig
	return &out, c.Do(ctx, http.MethodPatch, packageConfigPath(packageType), patch, &out)
}

// DeletePackageConfig fails with 409 packageInUse while customers use the package.
func (c *Client) DeletePackageConfig(ctx context.Context, packageType string) error {
	return c.Do(ctx, http.MethodDelete, packageConfigPath(packageType), nil, nil)
}

// SMTPSettings is the `smtp` object of GET/PATCH /api/organization/smtp.
type SMTPSettings struct {
	Host           *string `json:"host"`
	Port           *int64  `json:"port"`
	Username       *string `json:"username"`
	HasPassword    bool    `json:"hasPassword"`
	TLSMode        *string `json:"tlsMode"`
	FromName       *string `json:"fromName"`
	FromEmail      *string `json:"fromEmail"`
	ReplyTo        *string `json:"replyTo"`
	LastTestStatus *string `json:"lastTestStatus"`
}

func (c *Client) GetSMTP(ctx context.Context) (*SMTPSettings, error) {
	var out struct {
		SMTP SMTPSettings `json:"smtp"`
	}
	return &out.SMTP, c.Do(ctx, http.MethodGet, "/api/organization/smtp", nil, &out)
}

// UpdateSMTP PATCHes; {"disable": true} switches back to platform sending.
func (c *Client) UpdateSMTP(ctx context.Context, patch map[string]any) (*SMTPSettings, error) {
	var out struct {
		SMTP SMTPSettings `json:"smtp"`
	}
	return &out.SMTP, c.Do(ctx, http.MethodPatch, "/api/organization/smtp", patch, &out)
}

// WhitelabelDomain is { domain, dns } of POST and GET /api/organization/whitelabel/domains[/{id}].
type WhitelabelDomain struct {
	Domain struct {
		ID         int64   `json:"id"`
		Hostname   string  `json:"hostname"`
		Status     string  `json:"status"`
		IsPrimary  bool    `json:"isPrimary"`
		VerifiedAt *string `json:"verifiedAt"`
		CreatedAt  string  `json:"createdAt"`
		// Diagnosis of the last failed TXT verification, null after a successful one.
		LastVerificationError   json.RawMessage `json:"lastVerificationError"`
		LastVerificationErrorAt *string         `json:"lastVerificationErrorAt"`
	} `json:"domain"`
	DNS struct {
		TxtName  string `json:"txtName"`
		TxtValue string `json:"txtValue"`
	} `json:"dns"`
}

const whitelabelDomains = "/api/organization/whitelabel/domains"

func (c *Client) CreateWhitelabelDomain(ctx context.Context, hostname string) (*WhitelabelDomain, error) {
	var d WhitelabelDomain
	return &d, c.Do(ctx, http.MethodPost, whitelabelDomains, map[string]any{"hostname": hostname}, &d)
}

func (c *Client) GetWhitelabelDomain(ctx context.Context, id string) (*WhitelabelDomain, error) {
	var d WhitelabelDomain
	return &d, c.Do(ctx, http.MethodGet, whitelabelDomains+"/"+url.PathEscape(id), nil, &d)
}

func (c *Client) DeleteWhitelabelDomain(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, whitelabelDomains+"/"+url.PathEscape(id), nil, nil)
}

// ProblemSignals / ReportSections are the nested report objects (all keys required on write).
type ProblemSignals struct {
	Incident        bool   `json:"incident" tfsdk:"incident"`
	DowntimeMinutes *int64 `json:"downtimeMinutes" tfsdk:"downtime_minutes"`
	SslDaysLt       *int64 `json:"sslDaysLt" tfsdk:"ssl_days_lt"`
	ResponseBreach  bool   `json:"responseBreach" tfsdk:"response_breach"`
}

type ReportSections struct {
	FleetSummary    bool `json:"fleetSummary" tfsdk:"fleet_summary"`
	WorstPerformers bool `json:"worstPerformers" tfsdk:"worst_performers"`
	PerSiteTable    bool `json:"perSiteTable" tfsdk:"per_site_table"`
	IncidentLog     bool `json:"incidentLog" tfsdk:"incident_log"`
	SslExpiry       bool `json:"sslExpiry" tfsdk:"ssl_expiry"`
	GroupByCustomer bool `json:"groupByCustomer" tfsdk:"group_by_customer"`
}

// Report is GET /api/organization/reports/{id}.
type Report struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Enabled             bool                       `json:"enabled"`
	Frequency           string                     `json:"frequency"`
	Weekday             *int64                     `json:"weekday"`
	DayOfMonth          *int64                     `json:"dayOfMonth"`
	SendHour            int64                      `json:"sendHour"`
	Timezone            string                     `json:"timezone"`
	ScopeMode           string                     `json:"scopeMode"`
	ScopeCustomerIDs    []int64                    `json:"scopeCustomerIds"`
	ScopeTagIDs         []int64                    `json:"scopeTagIds"`
	InclusionMode       string                     `json:"inclusionMode"`
	ProblemSignals      ProblemSignals             `json:"problemSignals"`
	ThresholdUptimeLt   *float64                   `json:"thresholdUptimeLt"`
	ThresholdResponseGt *int64                     `json:"thresholdResponseGt"`
	SendWhenEmpty       bool                       `json:"sendWhenEmpty"`
	Sections            ReportSections             `json:"sections"`
	MonitorTypes        []string                   `json:"monitorTypes"`
	SectionsByType      map[string]map[string]bool `json:"sectionsByType"`
	GroupByCustomer     bool                       `json:"groupByCustomer"`
	Format              string                     `json:"format"`
	RecipientEmails     []string                   `json:"recipientEmails"`
	RecipientUserIDs    []string                   `json:"recipientUserIds"`
	CreatedAt           string                     `json:"createdAt"`
}

const reportsPath = "/api/organization/reports"

func (c *Client) CreateReport(ctx context.Context, body map[string]any) (*Report, error) {
	var r Report
	return &r, c.Do(ctx, http.MethodPost, reportsPath, body, &r)
}

func (c *Client) GetReport(ctx context.Context, id string) (*Report, error) {
	var r Report
	return &r, c.Do(ctx, http.MethodGet, reportsPath+"/"+url.PathEscape(id), nil, &r)
}

func (c *Client) UpdateReport(ctx context.Context, id string, patch map[string]any) (*Report, error) {
	var r Report
	return &r, c.Do(ctx, http.MethodPatch, reportsPath+"/"+url.PathEscape(id), patch, &r)
}

func (c *Client) DeleteReport(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, reportsPath+"/"+url.PathEscape(id), nil, nil)
}
