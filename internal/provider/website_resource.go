package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &websiteResource{}
	_ resource.ResourceWithImportState = &websiteResource{}
)

type websiteResource struct{ client *client.Client }

// websiteModel holds everything GET /api/websites/{id} returns; the data source uses it as is.
type websiteModel struct {
	ID                           types.String `tfsdk:"id"`
	PublicID                     types.String `tfsdk:"public_id"`
	CustomerID                   types.Int64  `tfsdk:"customer_id"`
	Name                         types.String `tfsdk:"name"`
	URL                          types.String `tfsdk:"url"`
	MonitoringType               types.String `tfsdk:"monitoring_type"`
	Status                       types.String `tfsdk:"status"`
	CheckInterval                types.Int64  `tfsdk:"check_interval"`
	TimeoutSeconds               types.Int64  `tfsdk:"timeout_seconds"`
	ExpectedStatusCodes          types.String `tfsdk:"expected_status_codes"`
	SearchTerm                   types.String `tfsdk:"search_term"`
	AllowedCheckCountryCodes     types.Set    `tfsdk:"allowed_check_country_codes"`
	AuthMode                     types.String `tfsdk:"auth_mode"`
	BasicAuthUsername            types.String `tfsdk:"basic_auth_username"`
	HTTPMethod                   types.String `tfsdk:"http_method"`
	FollowRedirects              types.Bool   `tfsdk:"follow_redirects"`
	CookieHandling               types.String `tfsdk:"cookie_handling"`
	ConnectHost                  types.String `tfsdk:"connect_host"`
	ConnectPort                  types.Int64  `tfsdk:"connect_port"`
	ConnectTLSInsecure           types.Bool   `tfsdk:"connect_tls_insecure"`
	CheckSSLEnabled              types.Bool   `tfsdk:"check_ssl_enabled"`
	CheckHTTPSRedirectEnabled    types.Bool   `tfsdk:"check_https_redirect_enabled"`
	CheckStatusEnabled           types.Bool   `tfsdk:"check_status_enabled"`
	CheckSizeEnabled             types.Bool   `tfsdk:"check_size_enabled"`
	CheckResponseTimeEnabled     types.Bool   `tfsdk:"check_response_time_enabled"`
	CheckKeywordEnabled          types.Bool   `tfsdk:"check_keyword_enabled"`
	CheckDomainExpiryEnabled     types.Bool   `tfsdk:"check_domain_expiry_enabled"`
	CheckExpectedResponseEnabled types.Bool   `tfsdk:"check_expected_response_enabled"`
	ExpectedResponseMatchType    types.String `tfsdk:"expected_response_match_type"`
	ExpectedResponseValue        types.String `tfsdk:"expected_response_value"`
	ExpectedResponseJSONPath     types.String `tfsdk:"expected_response_json_path"`
	SSLNoticeDays                types.Int64  `tfsdk:"ssl_notice_days"`
	SSLErrorDays                 types.Int64  `tfsdk:"ssl_error_days"`
	DomainExpiryNoticeDays       types.Int64  `tfsdk:"domain_expiry_notice_days"`
	DomainExpiryErrorDays        types.Int64  `tfsdk:"domain_expiry_error_days"`
	MinPageSize                  types.Int64  `tfsdk:"min_page_size"`
	MaxPageSize                  types.Int64  `tfsdk:"max_page_size"`
	HeartbeatToken               types.String `tfsdk:"heartbeat_token"`
	HeartbeatGracePeriodMinutes  types.Int64  `tfsdk:"heartbeat_grace_period_minutes"`
	HasCustomHeaders             types.Bool   `tfsdk:"has_custom_headers"`
	HasRequestBody               types.Bool   `tfsdk:"has_request_body"`
	CreatedAt                    types.String `tfsdk:"created_at"`
	UpdatedAt                    types.String `tfsdk:"updated_at"`
}

// websiteResourceModel adds the write-only secrets. They are never in plan or state (always
// null there) and are read from the configuration only.
type websiteResourceModel struct {
	websiteModel
	CustomHeadersWO        types.List   `tfsdk:"custom_headers_wo"`
	CustomHeadersWOVersion types.Int64  `tfsdk:"custom_headers_wo_version"`
	RequestBodyWO          types.String `tfsdk:"request_body_wo"`
	RequestBodyWOVersion   types.Int64  `tfsdk:"request_body_wo_version"`
	AuthorizationHeaderWO  types.String `tfsdk:"authorization_header_wo"`
	BasicAuthPasswordWO    types.String `tfsdk:"basic_auth_password_wo"`
	AuthWOVersion          types.Int64  `tfsdk:"auth_wo_version"`
}

type headerModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (m *websiteModel) fromAPI(ctx context.Context, w *client.Website) diag.Diagnostics {
	m.ID = types.StringValue(strconv.FormatInt(w.ID, 10))
	m.PublicID = types.StringValue(w.PublicID)
	m.CustomerID = types.Int64Value(w.CustomerID)
	m.Name = types.StringValue(w.Name)
	m.URL = types.StringValue(w.URL)
	m.MonitoringType = types.StringValue(w.MonitoringType)
	m.Status = types.StringValue(w.Status)
	m.CheckInterval = types.Int64Value(w.CheckInterval)
	m.TimeoutSeconds = types.Int64Value(w.TimeoutSeconds)
	m.ExpectedStatusCodes = types.StringPointerValue(w.ExpectedStatusCodes)
	m.SearchTerm = types.StringPointerValue(w.SearchTerm)
	m.AuthMode = types.StringValue(w.AuthMode)
	m.BasicAuthUsername = types.StringPointerValue(w.BasicAuthUsername)
	m.HTTPMethod = types.StringPointerValue(w.HTTPMethod)
	m.FollowRedirects = types.BoolPointerValue(w.FollowRedirects)
	m.CookieHandling = types.StringPointerValue(w.CookieHandling)
	m.ConnectHost = types.StringPointerValue(w.ConnectHost)
	m.ConnectPort = types.Int64PointerValue(w.ConnectPort)
	m.ConnectTLSInsecure = types.BoolValue(w.ConnectTLSInsecure)
	m.CheckSSLEnabled = types.BoolPointerValue(w.CheckSSLEnabled)
	m.CheckHTTPSRedirectEnabled = types.BoolPointerValue(w.CheckHTTPSRedirectEnabled)
	m.CheckStatusEnabled = types.BoolPointerValue(w.CheckStatusEnabled)
	m.CheckSizeEnabled = types.BoolPointerValue(w.CheckSizeEnabled)
	m.CheckResponseTimeEnabled = types.BoolPointerValue(w.CheckResponseTimeEnabled)
	m.CheckKeywordEnabled = types.BoolPointerValue(w.CheckKeywordEnabled)
	m.CheckDomainExpiryEnabled = types.BoolPointerValue(w.CheckDomainExpiryEnabled)
	m.CheckExpectedResponseEnabled = types.BoolPointerValue(w.CheckExpectedResponseEnabled)
	m.ExpectedResponseMatchType = types.StringPointerValue(w.ExpectedResponseMatchType)
	m.ExpectedResponseValue = types.StringPointerValue(w.ExpectedResponseValue)
	m.ExpectedResponseJSONPath = types.StringPointerValue(w.ExpectedResponseJSONPath)
	m.SSLNoticeDays = types.Int64PointerValue(w.SSLNoticeDays)
	m.SSLErrorDays = types.Int64PointerValue(w.SSLErrorDays)
	m.DomainExpiryNoticeDays = types.Int64PointerValue(w.DomainExpiryNoticeDays)
	m.DomainExpiryErrorDays = types.Int64PointerValue(w.DomainExpiryErrorDays)
	m.MinPageSize = types.Int64PointerValue(w.MinPageSize)
	m.MaxPageSize = types.Int64PointerValue(w.MaxPageSize)
	m.HeartbeatToken = types.StringPointerValue(w.HeartbeatToken)
	m.HeartbeatGracePeriodMinutes = types.Int64PointerValue(w.HeartbeatGracePeriodMinutes)
	m.HasCustomHeaders = types.BoolValue(w.HasCustomHeaders)
	m.HasRequestBody = types.BoolValue(w.HasRequestBody)
	m.CreatedAt = types.StringValue(w.CreatedAt)
	m.UpdatedAt = types.StringValue(w.UpdatedAt)
	if w.AllowedCheckCountryCodes == nil {
		m.AllowedCheckCountryCodes = types.SetNull(types.StringType)
		return nil
	}
	var d diag.Diagnostics
	m.AllowedCheckCountryCodes, d = types.SetValueFrom(ctx, types.StringType, w.AllowedCheckCountryCodes)
	return d
}

// websiteField maps one plain attribute to its API key. nullable: the API accepts null to clear
// it. partial: the partial PATCH mode accepts the key (partialFieldsSchema/partialCheckConfigSchema
// in server/api/websites/[id].patch.ts); it silently ignores every other key.
type websiteField struct {
	key               string
	get               func(*websiteModel) attr.Value
	nullable, partial bool
}

// Auth (auth_mode, basic_auth_username + secrets) is not in this list: it only goes out as a
// group, because authMode without its secret fails validation.
var websiteFields = []websiteField{
	{"customerId", func(m *websiteModel) attr.Value { return m.CustomerID }, false, false},
	{"name", func(m *websiteModel) attr.Value { return m.Name }, false, true},
	{"url", func(m *websiteModel) attr.Value { return m.URL }, false, true},
	{"monitoringType", func(m *websiteModel) attr.Value { return m.MonitoringType }, false, false},
	{"status", func(m *websiteModel) attr.Value { return m.Status }, false, true},
	{"checkInterval", func(m *websiteModel) attr.Value { return m.CheckInterval }, false, true},
	{"timeoutSeconds", func(m *websiteModel) attr.Value { return m.TimeoutSeconds }, false, true},
	{"expectedStatusCodes", func(m *websiteModel) attr.Value { return m.ExpectedStatusCodes }, false, true},
	{"searchTerm", func(m *websiteModel) attr.Value { return m.SearchTerm }, true, true},
	{"allowedCheckCountryCodes", func(m *websiteModel) attr.Value { return m.AllowedCheckCountryCodes }, true, false},
	{"httpMethod", func(m *websiteModel) attr.Value { return m.HTTPMethod }, false, false},
	{"followRedirects", func(m *websiteModel) attr.Value { return m.FollowRedirects }, false, false},
	{"cookieHandling", func(m *websiteModel) attr.Value { return m.CookieHandling }, false, false},
	{"connectHost", func(m *websiteModel) attr.Value { return m.ConnectHost }, true, true},
	{"connectPort", func(m *websiteModel) attr.Value { return m.ConnectPort }, true, true},
	{"connectTlsInsecure", func(m *websiteModel) attr.Value { return m.ConnectTLSInsecure }, false, true},
	{"checkSslEnabled", func(m *websiteModel) attr.Value { return m.CheckSSLEnabled }, false, true},
	{"checkHttpsRedirectEnabled", func(m *websiteModel) attr.Value { return m.CheckHTTPSRedirectEnabled }, false, true},
	{"checkStatusEnabled", func(m *websiteModel) attr.Value { return m.CheckStatusEnabled }, false, true},
	{"checkSizeEnabled", func(m *websiteModel) attr.Value { return m.CheckSizeEnabled }, false, true},
	{"checkResponseTimeEnabled", func(m *websiteModel) attr.Value { return m.CheckResponseTimeEnabled }, false, true},
	{"checkKeywordEnabled", func(m *websiteModel) attr.Value { return m.CheckKeywordEnabled }, false, true},
	{"checkDomainExpiryEnabled", func(m *websiteModel) attr.Value { return m.CheckDomainExpiryEnabled }, false, true},
	{"checkExpectedResponseEnabled", func(m *websiteModel) attr.Value { return m.CheckExpectedResponseEnabled }, false, true},
	{"expectedResponseMatchType", func(m *websiteModel) attr.Value { return m.ExpectedResponseMatchType }, false, true},
	{"expectedResponseValue", func(m *websiteModel) attr.Value { return m.ExpectedResponseValue }, true, true},
	{"expectedResponseJsonPath", func(m *websiteModel) attr.Value { return m.ExpectedResponseJSONPath }, true, true},
	{"sslNoticeDays", func(m *websiteModel) attr.Value { return m.SSLNoticeDays }, false, true},
	{"sslErrorDays", func(m *websiteModel) attr.Value { return m.SSLErrorDays }, false, true},
	{"domainExpiryNoticeDays", func(m *websiteModel) attr.Value { return m.DomainExpiryNoticeDays }, false, true},
	{"domainExpiryErrorDays", func(m *websiteModel) attr.Value { return m.DomainExpiryErrorDays }, false, true},
	{"minPageSize", func(m *websiteModel) attr.Value { return m.MinPageSize }, true, true},
	{"maxPageSize", func(m *websiteModel) attr.Value { return m.MaxPageSize }, true, true},
	{"heartbeatGracePeriodMinutes", func(m *websiteModel) attr.Value { return m.HeartbeatGracePeriodMinutes }, false, true},
}

// jsonValue turns a known framework value into its JSON form; null becomes nil.
func jsonValue(v attr.Value) any {
	if v.IsNull() {
		return nil
	}
	switch t := v.(type) {
	case types.String:
		return t.ValueString()
	case types.Int64:
		return t.ValueInt64()
	case types.Bool:
		return t.ValueBool()
	case types.Set:
		out := []string{}
		for _, e := range t.Elements() {
			out = append(out, e.(types.String).ValueString())
		}
		return out
	}
	return nil
}

// put sets key unless v is unknown, or null on a field the API does not accept null for.
func (f websiteField) put(body map[string]any, m *websiteModel) {
	v := f.get(m)
	if v.IsUnknown() || (v.IsNull() && !f.nullable) {
		return
	}
	body[f.key] = jsonValue(v)
}

// addSecrets puts the write-only groups into body, read from the configuration. create skips
// empty values; on update an empty value clears the stored secret.
func addSecrets(ctx context.Context, body map[string]any, cfg, plan *websiteResourceModel, auth, headers, reqBody, create bool) diag.Diagnostics {
	var diags diag.Diagnostics
	set := func(key string, v any, empty bool) {
		if !(create && empty) {
			body[key] = v
		}
	}
	if auth {
		if !plan.AuthMode.IsNull() && !plan.AuthMode.IsUnknown() {
			body["authMode"] = plan.AuthMode.ValueString()
		}
		set("basicAuthUsername", jsonValue(plan.BasicAuthUsername), plan.BasicAuthUsername.IsNull())
		set("authorizationHeader", jsonValue(cfg.AuthorizationHeaderWO), cfg.AuthorizationHeaderWO.IsNull())
		set("basicAuthPassword", jsonValue(cfg.BasicAuthPasswordWO), cfg.BasicAuthPasswordWO.IsNull())
	}
	if headers {
		var hs []headerModel
		diags.Append(cfg.CustomHeadersWO.ElementsAs(ctx, &hs, true)...)
		if len(hs) == 0 {
			set("customHeaders", nil, true)
		} else {
			m := map[string]string{}
			for _, h := range hs {
				m[h.Name.ValueString()] = h.Value.ValueString()
			}
			body["customHeaders"] = m
		}
	}
	if reqBody {
		set("requestBody", jsonValue(cfg.RequestBodyWO), cfg.RequestBodyWO.IsNull())
	}
	return diags
}

func NewWebsiteResource() resource.Resource { return &websiteResource{} }

func (r *websiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website"
}

func (r *websiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	optStr := func(desc, def string, v ...validator.String) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(def), Description: desc + " Defaults to `" + def + "`.", Validators: v}
	}
	optInt := func(desc string, def int64, v ...validator.Int64) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(def), Description: desc + " Defaults to `" + strconv.FormatInt(def, 10) + "`.", Validators: v}
	}
	// Server-chosen from the customer's package (or forced off for heartbeat monitors).
	pkgBool := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB, Description: desc + " Unset: the package default. The package may switch it off; a full update switches it off on heartbeat monitors."}
	}
	noCRLF := regexp.MustCompile(`^[^\r\n]*$`)

	resp.Schema = schema.Schema{
		Description: "A website monitor: HTTP(S) check (`combined`, `http_status`, `ssl_check`) or heartbeat (`heartbeat`, " +
			"the monitored system pings Uptimeify). Import with the numeric id or the public id.\n\n" +
			"Updates use the partial mode of `PATCH /api/websites/{id}` whenever every changed field is one that mode accepts " +
			"(name, url, status, interval, timeout, checks, keyword, expected response, connect target, SSL/domain days, page sizes, grace period). " +
			"Changes to the customer, countries, HTTP method, redirects, cookies, auth or the write-only secrets need the full mode, " +
			"which resets omitted fields to defaults, so the provider then sends every attribute and re-applies any value the full mode coerced " +
			"(`ssl_error_days = 0`, `min_page_size = 0`) with a partial PATCH.\n\n" +
			"Secrets (custom headers, request body, authorization header, basic auth password) are write-only: the API never returns them. " +
			"Bump the matching `*_wo_version` to send changed values.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Numeric website id."},
			"public_id":   schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Public UUID of the website."},
			"customer_id": schema.Int64Attribute{Required: true, Description: "Numeric id of the customer that owns the monitor. Changing it moves the monitor.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"name":        schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 255)}},
			"url": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "URL with protocol; required except for heartbeat monitors. Heartbeat monitors get `heartbeat://<token>` from the server, leave it unset there. Must resolve to a public address at create and update.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 2048)},
			},
			"monitoring_type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("combined"),
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description:   "`combined`, `http_status`, `ssl_check` or `heartbeat`. Changing it replaces the monitor. Defaults to `combined`.",
				Validators:    []validator.String{stringvalidator.OneOf("combined", "http_status", "ssl_check", "heartbeat")},
			},
			"status":                optStr("`active`, `inactive` or `maintenance`.", "active", stringvalidator.OneOf("active", "inactive", "maintenance")),
			"check_interval":        optInt("Minutes between checks; at least the package minimum, at most 1440 (43200 for heartbeat).", 30, int64validator.Between(1, 43200)),
			"timeout_seconds":       optInt("Request timeout, 1 to 60.", 30, int64validator.Between(1, 60)),
			"expected_status_codes": optStr("Comma-separated accepted status codes.", "200,301,302", stringvalidator.RegexMatches(regexp.MustCompile(`^[\d, ]+$`), "digits, commas and spaces")),
			"search_term": schema.StringAttribute{
				Optional: true, Description: "Keyword the page must contain (needs `check_keyword_enabled`; the server drops it while that check is off).",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"allowed_check_country_codes": schema.SetAttribute{
				ElementType: types.StringType, Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Description:   "Upper-case ISO country codes of the check locations. Unset at create: the package default (null = all). Removing it later keeps the last value.",
				Validators:    []validator.Set{setvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Z]{2}$`), "two upper-case letters"))},
			},
			"auth_mode":           optStr("`none`, `authorization_header` (needs `authorization_header_wo`) or `basic` (needs `basic_auth_username` and `basic_auth_password_wo`).", "none", stringvalidator.OneOf("none", "authorization_header", "basic")),
			"basic_auth_username": schema.StringAttribute{Optional: true, Description: "Username for `auth_mode = \"basic\"`.", Validators: []validator.String{stringvalidator.LengthBetween(1, 255)}},
			"authorization_header_wo": schema.StringAttribute{
				Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Write-only value of the Authorization header for `auth_mode = \"authorization_header\"`. Sent with create and whenever `auth_mode`, `basic_auth_username` or `auth_wo_version` changes.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 4096)},
			},
			"basic_auth_password_wo": schema.StringAttribute{
				Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Write-only basic auth password, sent like `authorization_header_wo`.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 4096)},
			},
			"auth_wo_version":  schema.Int64Attribute{Optional: true, Description: "Change to resend the auth secrets."},
			"http_method":      optStr("`GET`, `QUERY`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD` or `OPTIONS`.", "GET", stringvalidator.OneOf("GET", "QUERY", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS")),
			"follow_redirects": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Defaults to `true`."},
			"cookie_handling":  optStr("`none` or `jar` (keep cookies across redirects).", "none", stringvalidator.OneOf("none", "jar")),
			"custom_headers_wo": schema.ListNestedAttribute{
				Optional: true, WriteOnly: true,
				Description: "Write-only request headers. Names and values are stored encrypted and never returned (see `has_custom_headers`). Sent with create and whenever `custom_headers_wo_version` changes; an empty or removed list then clears them.",
				Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{Required: true, WriteOnly: true, Validators: []validator.String{
						stringvalidator.LengthBetween(1, 100), stringvalidator.RegexMatches(noCRLF, "no line breaks")}},
					"value": schema.StringAttribute{Required: true, WriteOnly: true, Sensitive: true, Validators: []validator.String{
						stringvalidator.LengthAtMost(8192), stringvalidator.RegexMatches(noCRLF, "no line breaks")}},
				}},
			},
			"custom_headers_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `custom_headers_wo`."},
			"request_body_wo": schema.StringAttribute{
				Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Write-only request body (stored encrypted, never returned, see `has_request_body`). Sent with create and whenever `request_body_wo_version` changes.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 100000)},
			},
			"request_body_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `request_body_wo`."},
			"connect_host": schema.StringAttribute{
				Optional:    true,
				Description: "Connect target (`connectHost`): IP or host name to connect to instead of the URL's host; SNI and Host header stay the URL's. Lower case, without port (use `connect_port`), must be public.",
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`^([a-z0-9_*.-]*[a-z0-9_*-]|[0-9a-f.]*:[0-9a-f.]*:[0-9a-f:.]*)$`), "a lower-case host or IP without port")},
			},
			"connect_port":         schema.Int64Attribute{Optional: true, Description: "Port of the connect target; unset = the URL's port.", Validators: []validator.Int64{int64validator.Between(1, 65535)}},
			"connect_tls_insecure": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Skip certificate verification at the connect target; needs `connect_host`. Defaults to `false`."},

			"check_ssl_enabled":            pkgBool("Certificate validity and expiry check."),
			"check_https_redirect_enabled": pkgBool("HTTP to HTTPS redirect check."),
			"check_status_enabled":         pkgBool("Status code check against `expected_status_codes`."),
			"check_size_enabled":           pkgBool("Page size check against `min_page_size`/`max_page_size`; off without a limit."),
			"check_response_time_enabled":  pkgBool("Response time check (the threshold is not configurable per monitor)."),
			"check_keyword_enabled":        pkgBool("Keyword check against `search_term`; off without a search term."),
			"check_domain_expiry_enabled":  pkgBool("Domain expiry check."),
			"check_expected_response_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Compare the response body with `expected_response_value`. Defaults to `false`."},
			"expected_response_match_type": optStr("`contains`, `equals` or `json_path_equals` (needs `expected_response_json_path`).", "contains", stringvalidator.OneOf("contains", "equals", "json_path_equals")),
			"expected_response_value":      schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 10000)}},
			"expected_response_json_path":  schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 500)}},
			"ssl_notice_days":              optInt("Warn this many days before certificate expiry.", 7, int64validator.Between(1, 365)),
			"ssl_error_days":               optInt("Error this many days before certificate expiry; at most `ssl_notice_days`.", 0, int64validator.Between(0, 365)),
			"domain_expiry_notice_days":    optInt("Warn this many days before domain expiry.", 30, int64validator.Between(1, 365)),
			"domain_expiry_error_days":     optInt("Error this many days before domain expiry.", 7, int64validator.Between(0, 365)),
			"min_page_size":                schema.Int64Attribute{Optional: true, Description: "Bytes; dropped by the server while `check_size_enabled` is off.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"max_page_size":                schema.Int64Attribute{Optional: true, Description: "Bytes; dropped by the server while `check_size_enabled` is off.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"heartbeat_token":              schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Heartbeat monitors: token of the ping URL (server-generated)."},
			"heartbeat_grace_period_minutes": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Heartbeat monitors: minutes a ping may be late. Server default 5; null for other types.", Validators: []validator.Int64{int64validator.Between(1, 10080)}},
			"has_custom_headers": schema.BoolAttribute{Computed: true, Description: "Whether custom headers are stored."},
			"has_request_body":   schema.BoolAttribute{Computed: true, Description: "Whether a request body is stored."},
			"created_at":         schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"updated_at":         schema.StringAttribute{Computed: true},
		},
	}
}

func (r *websiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *websiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg websiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{}
	for _, f := range websiteFields {
		if v := f.get(&plan.websiteModel); !v.IsNull() {
			f.put(body, &plan.websiteModel)
		}
	}
	resp.Diagnostics.Append(addSecrets(ctx, body, &cfg, &plan, true, true, true, true)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.CreateWebsite(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating website failed", err.Error())
		return
	}
	got := plan
	r.readInto(ctx, id, &got, &resp.Diagnostics)
	resp.Diagnostics.Append(clampedChecks(&plan.websiteModel, &got.websiteModel)...)
	// Stored even with a clamp error, so the created monitor is tracked and not orphaned.
	resp.Diagnostics.Append(resp.State.Set(ctx, &got)...)
}

// websiteChecks are the package-gated check switches the server may silently switch off.
var websiteChecks = []struct {
	name string
	get  func(*websiteModel) types.Bool
}{
	{"check_ssl_enabled", func(m *websiteModel) types.Bool { return m.CheckSSLEnabled }},
	{"check_https_redirect_enabled", func(m *websiteModel) types.Bool { return m.CheckHTTPSRedirectEnabled }},
	{"check_status_enabled", func(m *websiteModel) types.Bool { return m.CheckStatusEnabled }},
	{"check_size_enabled", func(m *websiteModel) types.Bool { return m.CheckSizeEnabled }},
	{"check_response_time_enabled", func(m *websiteModel) types.Bool { return m.CheckResponseTimeEnabled }},
	{"check_keyword_enabled", func(m *websiteModel) types.Bool { return m.CheckKeywordEnabled }},
	{"check_domain_expiry_enabled", func(m *websiteModel) types.Bool { return m.CheckDomainExpiryEnabled }},
}

// clampedChecks turns "planned true, server stored false" into a clear error instead of the
// framework's "inconsistent result" (clampWebsiteCheckColumns/dropDeadCompanionChecks on the server).
func clampedChecks(want, got *websiteModel) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, c := range websiteChecks {
		if w := c.get(want); !w.IsUnknown() && w.ValueBool() && !c.get(got).ValueBool() {
			diags.AddAttributeError(path.Root(c.name), "Check not allowed",
				fmt.Sprintf("%s = true is not allowed by the customer's package (server switched it off). "+
					"Set it to false or leave it unset. The keyword and size checks also switch off without "+
					"search_term or page size limits, and full updates switch the standard checks off on heartbeat monitors.", c.name))
		}
	}
	return diags
}

// readInto reads the website back with GET (which, unlike the POST/PATCH answer, carries
// hasCustomHeaders/hasRequestBody) and copies it into m.
func (r *websiteResource) readInto(ctx context.Context, id string, m *websiteResourceModel, diags *diag.Diagnostics) {
	w, err := r.client.GetWebsite(ctx, id)
	if err != nil {
		diags.AddError("Reading website failed", err.Error())
		return
	}
	diags.Append(m.fromAPI(ctx, w)...)
}

func (r *websiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetWebsite(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading website failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, w)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg websiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	auth := !plan.AuthMode.Equal(state.AuthMode) || !plan.BasicAuthUsername.Equal(state.BasicAuthUsername) || !plan.AuthWOVersion.Equal(state.AuthWOVersion)
	headers := !plan.CustomHeadersWOVersion.Equal(state.CustomHeadersWOVersion)
	reqBody := !plan.RequestBodyWOVersion.Equal(state.RequestBodyWOVersion)
	body, full := websiteUpdateBody(&plan.websiteModel, &state.websiteModel, auth || headers || reqBody)
	if full {
		resp.Diagnostics.Append(addSecrets(ctx, body, &cfg, &plan, auth, headers, reqBody, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if len(body) > 0 {
		if err := r.client.UpdateWebsite(ctx, id, body); err != nil {
			resp.Diagnostics.AddError("Updating website failed", err.Error())
			return
		}
	}
	got := plan
	r.readInto(ctx, id, &got, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if fix := partialBody(&plan.websiteModel, &got.websiteModel); full && len(fix) > 0 {
		if err := r.client.UpdateWebsite(ctx, id, fix); err != nil {
			resp.Diagnostics.AddError("Updating website failed (re-applying values the full update coerced)", err.Error())
			return
		}
		r.readInto(ctx, id, &got, &resp.Diagnostics)
	}
	resp.Diagnostics.Append(clampedChecks(&plan.websiteModel, &got.websiteModel)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &got)...)
}

// websiteUpdateBody chooses the PATCH mode (api-standards §27 B9). Partial mode writes only the
// keys it is sent, but accepts only a subset of fields and silently drops the rest. Full mode
// (customerId + name + url in the body) accepts every field but resets omitted ones (status to
// active, interval to 30, search term to null, ...), so it gets every attribute. Auth and the
// write-only secrets exist only in full mode.
func websiteUpdateBody(plan, state *websiteModel, secretsChanged bool) (map[string]any, bool) {
	full := secretsChanged
	for _, f := range websiteFields {
		if !f.partial && !f.get(plan).Equal(f.get(state)) {
			full = true
		}
	}
	if !full {
		return partialBody(plan, state), false
	}
	body := map[string]any{}
	for _, f := range websiteFields {
		f.put(body, plan)
	}
	return body, true
}

// partialBody holds the partial-mode keys whose value in want differs from have. The partial
// mode reads connectPort only together with connectHost, so a port change sends both.
func partialBody(want, have *websiteModel) map[string]any {
	body := map[string]any{}
	for _, f := range websiteFields {
		if f.partial && !f.get(want).IsUnknown() && !f.get(want).Equal(f.get(have)) {
			f.put(body, want)
		}
	}
	if _, ok := body["connectPort"]; ok {
		body["connectHost"] = jsonValue(want.ConnectHost)
	}
	return body
}

func (r *websiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state websiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteWebsite(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting website failed", err.Error())
	}
}

func (r *websiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
