package provider

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// API parity: every request-body field of the endpoints below is either an attribute of the
// resource that writes it, an alias in parityAliases, or listed in parityIgnored with a reason.
// A new field in the app's API contract therefore fails this test until the provider maps it or
// says why not.
//
// The fields come from testdata/api_request_fields.json, a snapshot of the app's generated
// OpenAPI document (website-monitoring: monitoring-app/server/assets/openapi/openapi.json).
// Refresh: run this test with UPTIMEIFY_OPENAPI=<path to openapi.json> (see CONTRIBUTING.md),
// then review and commit the snapshot diff.

const paritySnapshot = "testdata/api_request_fields.json"

var parityEndpoints = map[string]func() resource.Resource{
	"POST /api/status-pages":                                NewStatusPageResource,
	"PATCH /api/status-pages/{id}":                          NewStatusPageResource,
	"PATCH /api/status-pages/{id}/design":                   NewStatusPageDesignResource,
	"POST /api/status-pages/{id}/components":                NewStatusPageComponentResource,
	"PATCH /api/status-pages/{id}/components/{componentId}": NewStatusPageComponentResource,
	"POST /api/status-pages/{id}/groups":                    NewStatusPageComponentGroupResource,
	"PATCH /api/status-pages/{id}/groups/{groupId}":         NewStatusPageComponentGroupResource,
	"POST /api/status-pages/maintenances":                   NewStatusPageMaintenanceResource,
	"PATCH /api/status-pages/maintenances/{id}":             NewStatusPageMaintenanceResource,
	"POST /api/websites":                                    NewWebsiteResource,
	"PATCH /api/websites/{id}":                              NewWebsiteResource,
	"POST /api/customers":                                   NewCustomerResource,
	"PATCH /api/customers/{id}":                             NewCustomerResource,
	"POST /api/tags":                                        NewTagResource,
	"PATCH /api/tags/{id}":                                  NewTagResource,
	"POST /api/maintenance-windows":                         NewMaintenanceWindowResource,
	"PATCH /api/maintenance-windows/{id}":                   NewMaintenanceWindowResource,
	"POST /api/notification-channels":                       NewNotificationChannelResource,
	"PATCH /api/notification-channels/{id}":                 NewNotificationChannelResource,
	"PATCH /api/organization/smtp":                          NewSMTPSettingsResource,
	"PATCH /api/organization/whitelabel/branding":           NewBrandingResource,
	// PUT /api/organization/sso (uptimeify_sso_oidc) is not in the OpenAPI document yet.
	"POST /api/dns-monitors":                           NewDNSMonitorResource,
	"PATCH /api/dns-monitors/{id}":                     NewDNSMonitorResource,
	"POST /api/tcp-monitors":                           NewTCPMonitorResource,
	"PATCH /api/tcp-monitors/{id}":                     NewTCPMonitorResource,
	"POST /api/icmp-monitors":                          NewICMPMonitorResource,
	"PATCH /api/icmp-monitors/{id}":                    NewICMPMonitorResource,
	"POST /api/smtp-monitors":                          NewSMTPMonitorResource,
	"PATCH /api/smtp-monitors/{id}":                    NewSMTPMonitorResource,
	"POST /api/ssh-monitors":                           NewSSHMonitorResource,
	"PATCH /api/ssh-monitors/{id}":                     NewSSHMonitorResource,
	"POST /api/ftp-monitors":                           NewFTPMonitorResource,
	"PATCH /api/ftp-monitors/{id}":                     NewFTPMonitorResource,
	"POST /api/imap-pop-monitors":                      NewIMAPPopMonitorResource,
	"PATCH /api/imap-pop-monitors/{id}":                NewIMAPPopMonitorResource,
	"PUT /api/im/users/{userId}/notification-settings": NewUserNotificationSettingsResource,
	"PUT /api/im/notification-template":                NewNotificationTemplateResource,
}

// parityAliases maps "<endpoint> <apiField>" (or "* <apiField>" for every endpoint) to the
// attribute that carries it when the snake_case form differs.
var parityAliases = map[string]string{
	// Secrets are write-only attributes (`*_wo` plus a `*_wo_version` trigger).
	"* accessPassword":                      "access_password_wo",
	"* authorizationHeader":                 "authorization_header_wo",
	"* basicAuthPassword":                   "basic_auth_password_wo",
	"* customHeaders":                       "custom_headers_wo",
	"* requestBody":                         "request_body_wo",
	"PATCH /api/organization/smtp password": "password_wo",
	// One target list instead of one id field per monitor kind; recurrence is its own attribute.
	"* websiteId":        "targets",
	"* dnsMonitorId":     "targets",
	"* ftpMonitorId":     "targets",
	"* icmpMonitorId":    "targets",
	"* imapPopMonitorId": "targets",
	"* smtpMonitorId":    "targets",
	"* sshMonitorId":     "targets",
	"* tcpMonitorId":     "targets",
	"* customerDomainId": "targets",
	"* customerIpId":     "targets",
	"* isRecurring":      "recurrence_pattern",
}

// parityIgnored: "<endpoint> <apiField>" (or "* <apiField>") -> why the provider does not send it.
var parityIgnored = map[string]string{
	"* notificationChannels":                             "deprecated per-customer channel switches, the API asks for null",
	"* showRecentIncidents":                              "not mapped yet: status page incident/maintenance history toggles",
	"* showRecentMaintenance":                            "not mapped yet: status page incident/maintenance history toggles",
	"POST /api/status-pages customDomainHostname":        "uptimeify_status_page_domain owns the custom domain",
	"PATCH /api/status-pages/{id} designConfig":          "uptimeify_status_page_design owns the design",
	"PATCH /api/status-pages/{id} monitorUpdatesEnabled": "not mapped yet: monitors writing to the page (app 4bfb5a63d)",
	"PATCH /api/status-pages/{id} audienceGroups":        "uptimeify_status_page_audience_groups owns them (they name components, which need the page first)",
	"PATCH /api/organization/smtp disable":               "destroy of uptimeify_smtp_settings sends it",
	"PATCH /api/organization/smtp clearLastTest":         "UI bookkeeping of the SMTP test mail",
	"POST /api/notification-channels sourceChannelId":    "UI duplicate action, no lasting state",
	"POST /api/notification-channels websiteId":          "legacy per-website channel; channels attach via notification_targets/conditions",
	// Protocol monitors: the nested config object (config or <type>Config) is flattened into the
	// type's own attributes (protocol_monitor_types.go); its keys are not part of this check.
	"* config":                           "nested config object, flattened into the type's attributes",
	"* dnsConfig":                        "nested config object, flattened into the type's attributes",
	"* ftpConfig":                        "nested config object, flattened into the type's attributes",
	"* icmpConfig":                       "nested config object, flattened into the type's attributes",
	"* imapPopConfig":                    "nested config object, flattened into the type's attributes",
	"* smtpConfig":                       "nested config object, flattened into the type's attributes",
	"* sshConfig":                        "nested config object, flattened into the type's attributes",
	"PATCH /api/icmp-monitors/{id} port": "ICMP has no port; the shared PATCH schema accepts it",
	"PATCH /api/organization/whitelabel/branding themeNeutral": "deprecated alias of themeSecondary (until 2027-02-01)",
	"* managementType":                            "not mapped yet: managed vs self-service website (customer self-service)",
	"POST /api/websites allowPaidQuotaUpgrade":    "one-time consent for a paid quota upgrade, not state",
	"POST /api/websites customFields":             "not mapped yet: website custom field values",
	"POST /api/websites dnsConfig":                "not mapped yet: DNS sub-check of the website create",
	"POST /api/websites mtlsEnabled":              "not mapped yet: mTLS client certificate",
	"POST /api/websites mtlsClientCert":           "not mapped yet: mTLS client certificate",
	"POST /api/websites mtlsClientKey":            "not mapped yet: mTLS client certificate",
	"POST /api/websites playwrightScript":         "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightEnv":            "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightDevice":         "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightRetries":        "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightTimeoutMs":      "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightViewportHeight": "not mapped yet: browser (Playwright) monitors",
	"POST /api/websites playwrightViewportWidth":  "not mapped yet: browser (Playwright) monitors",
}

var camelRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func snake(s string) string { return strings.ToLower(camelRe.ReplaceAllString(s, "${1}_${2}")) }

func TestAPIParity(t *testing.T) {
	if p := os.Getenv("UPTIMEIFY_OPENAPI"); p != "" {
		writeParitySnapshot(t, p)
	}
	raw, err := os.ReadFile(paritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string][]string
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for ep, newRes := range parityEndpoints {
		fields, ok := snap[ep]
		if !ok {
			t.Errorf("%s: not in %s (endpoint gone or renamed? refresh the snapshot)", ep, paritySnapshot)
			continue
		}
		var sch resource.SchemaResponse
		newRes().Schema(ctx, resource.SchemaRequest{}, &sch)
		for _, f := range fields {
			attr := snake(f)
			if a, ok := parityAliases[ep+" "+f]; ok {
				attr = a
			} else if a, ok := parityAliases["* "+f]; ok {
				attr = a
			}
			if _, ok := sch.Schema.Attributes[attr]; ok {
				continue
			}
			if _, ok := sch.Schema.Blocks[attr]; ok {
				continue
			}
			if parityIgnored[ep+" "+f] != "" || parityIgnored["* "+f] != "" {
				continue
			}
			t.Errorf("%s: request field %q has no attribute %q; map it, alias it or add it to parityIgnored with a reason", ep, f, attr)
		}
	}
}

// writeParitySnapshot extracts the top-level request-body fields of parityEndpoints from an OpenAPI document.
func writeParitySnapshot(t *testing.T, openapiPath string) {
	t.Helper()
	raw, err := os.ReadFile(openapiPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	type sch struct {
		Ref        string                     `json:"$ref"`
		Properties map[string]json.RawMessage `json:"properties"`
		AllOf      []json.RawMessage          `json:"allOf"`
		OneOf      []json.RawMessage          `json:"oneOf"`
		AnyOf      []json.RawMessage          `json:"anyOf"`
	}
	var props func(json.RawMessage, map[string]bool)
	props = func(m json.RawMessage, into map[string]bool) {
		var s sch
		if json.Unmarshal(m, &s) != nil {
			return
		}
		if s.Ref != "" {
			props(doc.Components.Schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")], into)
		}
		for k := range s.Properties {
			into[k] = true
		}
		for _, l := range [][]json.RawMessage{s.AllOf, s.OneOf, s.AnyOf} {
			for _, x := range l {
				props(x, into)
			}
		}
	}
	out := map[string][]string{}
	for ep := range parityEndpoints {
		method, p, _ := strings.Cut(ep, " ")
		var op struct {
			RequestBody struct {
				Content map[string]struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		}
		opRaw, ok := doc.Paths[p][strings.ToLower(method)]
		if !ok {
			t.Errorf("%s: not in the OpenAPI document", ep)
			continue
		}
		_ = json.Unmarshal(opRaw, &op)
		set := map[string]bool{}
		props(op.RequestBody.Content["application/json"].Schema, set)
		fields := []string{}
		for k := range set {
			fields = append(fields, k)
		}
		sort.Strings(fields)
		out[ep] = fields
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(paritySnapshot, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
