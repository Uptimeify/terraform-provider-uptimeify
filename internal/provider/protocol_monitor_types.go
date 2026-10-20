package provider

import (
	"context"
	"maps"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Per type: model (monitorBase + own fields), attributes, body mapping (put) and row mapping (read).
// Contract: monitoring-app/server/lib/api-contract/protocol-monitors.ts; config keys are the ones
// the workers read (monitoring-workers/src/lib/probes/*.ts).

func NewDNSMonitorResource() resource.Resource {
	return &monitorResource[dnsMonitorModel, *dnsMonitorModel]{spec: dnsMonitorSpec}
}
func NewDNSMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[dnsMonitorModel, *dnsMonitorModel]{spec: dnsMonitorSpec}
}
func NewTCPMonitorResource() resource.Resource {
	return &monitorResource[tcpMonitorModel, *tcpMonitorModel]{spec: tcpMonitorSpec}
}
func NewTCPMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[tcpMonitorModel, *tcpMonitorModel]{spec: tcpMonitorSpec}
}
func NewICMPMonitorResource() resource.Resource {
	return &monitorResource[icmpMonitorModel, *icmpMonitorModel]{spec: icmpMonitorSpec}
}
func NewICMPMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[icmpMonitorModel, *icmpMonitorModel]{spec: icmpMonitorSpec}
}
func NewSMTPMonitorResource() resource.Resource {
	return &monitorResource[smtpMonitorModel, *smtpMonitorModel]{spec: smtpMonitorSpec}
}
func NewSMTPMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[smtpMonitorModel, *smtpMonitorModel]{spec: smtpMonitorSpec}
}
func NewSSHMonitorResource() resource.Resource {
	return &monitorResource[sshMonitorModel, *sshMonitorModel]{spec: sshMonitorSpec}
}
func NewSSHMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[sshMonitorModel, *sshMonitorModel]{spec: sshMonitorSpec}
}
func NewFTPMonitorResource() resource.Resource {
	return &monitorResource[ftpMonitorModel, *ftpMonitorModel]{spec: ftpMonitorSpec}
}
func NewFTPMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[ftpMonitorModel, *ftpMonitorModel]{spec: ftpMonitorSpec}
}
func NewIMAPPopMonitorResource() resource.Resource {
	return &monitorResource[imapPopMonitorModel, *imapPopMonitorModel]{spec: imapPopMonitorSpec}
}
func NewIMAPPopMonitorDataSource() datasource.DataSource {
	return &monitorDataSource[imapPopMonitorModel, *imapPopMonitorModel]{spec: imapPopMonitorSpec}
}

// ---- helpers -----------------------------------------------------------------------------------

// anyDiffers takes plan/prior pairs and reports whether an update changes one of them.
func (b *bodyBuilder) anyDiffers(pairs ...attr.Value) bool {
	for i := 0; i+1 < len(pairs); i += 2 {
		if b.differs(pairs[i], pairs[i+1]) {
			return true
		}
	}
	return false
}

// putSecret sets key to a write-only value: on create only when set (the strict create schemas
// reject null), on update always (null clears the stored secret).
func putSecret(b *bodyBuilder, into map[string]any, key string, v types.String) {
	if b.create && v.IsNull() {
		return
	}
	into[key] = v.ValueStringPointer()
}

func cfgMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func cfgString(v any) types.String {
	if s, ok := v.(string); ok && s != "" {
		return types.StringValue(s)
	}
	return types.StringNull()
}

func cfgBool(v any, def bool) types.Bool {
	if b, ok := v.(bool); ok {
		return types.BoolValue(b)
	}
	return types.BoolValue(def)
}

func cfgInt(v any) types.Int64 {
	if f, ok := v.(float64); ok {
		return types.Int64Value(int64(f))
	}
	return types.Int64Null()
}

// keepBool is for tcp check mode, where the API stores no protocol config: keep what the
// configuration says, or the default when nothing is known (import).
func keepBool(cur types.Bool, def bool) types.Bool {
	if cur.IsNull() || cur.IsUnknown() {
		return types.BoolValue(def)
	}
	return cur
}

func keepString(cur types.String, def string) types.String {
	if cur.IsNull() || cur.IsUnknown() {
		return types.StringValue(def)
	}
	return cur
}

func boolAttr(desc string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(def), Description: desc}
}

func writeOnly(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
		Description: desc + " Write-only (Terraform 1.11+ / OpenTofu 1.11+), never stored in state or returned by the API; sent on create and whenever the matching `*_wo_version` changes."}
}

func woVersion(what string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Description: "Change this number to send " + what + " again."}
}

// ---- protocol port + check mode (smtp, ssh, ftp, imap-pop) --------------------------------------

type protocolPort struct {
	Port      types.Int64  `tfsdk:"port"`
	CheckMode types.String `tfsdk:"check_mode"`
}

func protocolPortAttrs(defaultPort string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"port": schema.Int64Attribute{
			Optional:    true,
			Description: "TCP port. Unset = " + defaultPort + ".",
			Validators:  []validator.Int64{int64validator.Between(1, 65535)},
		},
		"check_mode": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("protocol"),
			Description: "`protocol` (full login check, credentials required) or `tcp` (port reachability only; the API then stores no protocol settings and no credentials). Defaults to `protocol`.",
			Validators:  []validator.String{stringvalidator.OneOf("protocol", "tcp")},
		},
	}
}

// putPort adds port and checkMode. protocol = the config is used at all; full = the request must
// carry the whole config and every secret: on create, and when leaving tcp mode (which wiped them).
func (p *protocolPort) putPort(b *bodyBuilder, prior *protocolPort) (protocol, full bool) {
	b.put("port", p.Port, prior.Port, p.Port.ValueInt64Pointer())
	b.put("checkMode", p.CheckMode, prior.CheckMode, p.CheckMode.ValueString())
	return p.CheckMode.ValueString() != "tcp", b.create || prior.CheckMode.ValueString() == "tcp"
}

// readPort reports whether the row is in tcp mode (config empty, keep the configured values).
func (p *protocolPort) readPort(row *client.MonitorRow) (tcpMode bool) {
	p.Port = types.Int64PointerValue(row.Port)
	p.CheckMode = types.StringValue(row.CheckMode)
	return row.CheckMode == "tcp"
}

func merge(parts ...map[string]schema.Attribute) map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, p := range parts {
		maps.Copy(out, p)
	}
	return out
}

// ---- DNS ---------------------------------------------------------------------------------------

type dnsMonitorModel struct {
	monitorBase
	RecordTypes           types.List   `tfsdk:"record_types"`
	MatchMode             types.String `tfsdk:"match_mode"`
	ExpectedValues        types.Map    `tfsdk:"expected_values"`
	TriggerOnResolveError types.Bool   `tfsdk:"trigger_on_resolve_error"`
	TriggerOnMismatch     types.Bool   `tfsdk:"trigger_on_mismatch"`
	RequireExpectedValues types.Bool   `tfsdk:"require_expected_values"`
}

var rrType = regexp.MustCompile(`^[A-Z0-9]+$`)

var dnsMonitorSpec = monitorSpec{
	name:        "dns_monitor",
	kind:        "dns-monitors",
	title:       "DNS monitor",
	description: "A DNS monitor: resolves `hostname` and compares the answers per record type with `expected_values`.",
	attrs: map[string]schema.Attribute{
		"record_types": schema.ListAttribute{
			ElementType: types.StringType,
			Required:    true,
			Description: "Record types to resolve, upper case and unique, e.g. `[\"A\", \"MX\"]`.",
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				listvalidator.UniqueValues(),
				listvalidator.ValueStringsAre(stringvalidator.RegexMatches(rrType, "must be an upper-case record type")),
			},
		},
		"match_mode": schema.StringAttribute{
			Required:    true,
			Description: "`exact` (answer set equals the expected values) or `contains` (answer includes them).",
			Validators:  []validator.String{stringvalidator.OneOf("exact", "contains")},
		},
		"expected_values": schema.MapAttribute{
			ElementType: types.ListType{ElemType: types.StringType},
			Required:    true,
			Description: "Expected answers per record type (upper-case key); every type in `record_types` needs at least one value. Values unique and without surrounding spaces.",
			Validators: []validator.Map{
				mapvalidator.KeysAre(stringvalidator.RegexMatches(rrType, "must be an upper-case record type")),
				mapvalidator.ValueListsAre(
					listvalidator.SizeAtLeast(1),
					listvalidator.UniqueValues(),
					listvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(`^\S(.*\S)?$`), "must not have surrounding spaces")),
				),
			},
		},
		"trigger_on_resolve_error": boolAttr("Alert when the name does not resolve. Defaults to true.", true),
		"trigger_on_mismatch":      boolAttr("Alert when the answers do not match. Defaults to true.", true),
		"require_expected_values":  boolAttr("Defaults to false.", false),
	},
}

func (m *dnsMonitorModel) put(ctx context.Context, b *bodyBuilder, _, prior *dnsMonitorModel) diag.Diagnostics {
	// PATCH replaces dnsConfig as a whole.
	if !b.create && !b.anyDiffers(m.RecordTypes, prior.RecordTypes, m.MatchMode, prior.MatchMode,
		m.ExpectedValues, prior.ExpectedValues, m.TriggerOnResolveError, prior.TriggerOnResolveError,
		m.TriggerOnMismatch, prior.TriggerOnMismatch, m.RequireExpectedValues, prior.RequireExpectedValues) {
		return nil
	}
	var diags diag.Diagnostics
	var rr []string
	var ev map[string][]string
	diags.Append(m.RecordTypes.ElementsAs(ctx, &rr, false)...)
	diags.Append(m.ExpectedValues.ElementsAs(ctx, &ev, false)...)
	b.body["dnsConfig"] = map[string]any{
		"rrtypes":        rr,
		"matchMode":      m.MatchMode.ValueString(),
		"expectedValues": ev,
		"triggerOn": map[string]any{
			"resolveError": m.TriggerOnResolveError.ValueBool(),
			"mismatch":     m.TriggerOnMismatch.ValueBool(),
		},
		"requireExpectedValues": m.RequireExpectedValues.ValueBool(),
	}
	return diags
}

func (m *dnsMonitorModel) read(ctx context.Context, row *client.MonitorRow) diag.Diagnostics {
	c := row.DNSConfig
	if c == nil {
		c = &client.DNSConfig{}
	}
	var diags, d diag.Diagnostics
	m.RecordTypes, d = types.ListValueFrom(ctx, types.StringType, c.RRTypes)
	diags.Append(d...)
	m.ExpectedValues, d = types.MapValueFrom(ctx, types.ListType{ElemType: types.StringType}, c.ExpectedValues)
	diags.Append(d...)
	m.MatchMode = types.StringValue(c.MatchMode)
	m.TriggerOnResolveError, m.TriggerOnMismatch = types.BoolValue(true), types.BoolValue(true)
	if t := c.TriggerOn; t != nil {
		if t.ResolveError != nil {
			m.TriggerOnResolveError = types.BoolValue(*t.ResolveError)
		}
		if t.Mismatch != nil {
			m.TriggerOnMismatch = types.BoolValue(*t.Mismatch)
		}
	}
	m.RequireExpectedValues = types.BoolValue(c.RequireExpectedValues != nil && *c.RequireExpectedValues)
	return diags
}

// ---- TCP ---------------------------------------------------------------------------------------

type tcpMonitorModel struct {
	monitorBase
	Port         types.Int64  `tfsdk:"port"`
	ExpectBanner types.String `tfsdk:"expect_banner"`
}

var tcpMonitorSpec = monitorSpec{
	name:        "tcp_monitor",
	kind:        "tcp-monitors",
	title:       "TCP monitor",
	description: "A TCP monitor: connects to `hostname`:`port`, optionally expecting a banner. The hostname must resolve to a public address when it is set.",
	attrs: map[string]schema.Attribute{
		"port": schema.Int64Attribute{
			Required:   true,
			Validators: []validator.Int64{int64validator.Between(1, 65535)},
		},
		"expect_banner": schema.StringAttribute{
			Optional:    true,
			Description: "Text the server's greeting must contain, at most 255 characters.",
			Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
		},
	},
}

func (m *tcpMonitorModel) put(_ context.Context, b *bodyBuilder, _, prior *tcpMonitorModel) diag.Diagnostics {
	b.put("port", m.Port, prior.Port, m.Port.ValueInt64())
	if b.changed(m.ExpectBanner, prior.ExpectBanner) {
		// PATCH merges config; "" removes the banner (the API has no way to delete the key).
		b.body["config"] = map[string]any{"expectBanner": m.ExpectBanner.ValueString()}
	}
	return nil
}

func (m *tcpMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	m.Port = types.Int64PointerValue(row.Port)
	m.ExpectBanner = cfgString(row.Config["expectBanner"])
	return nil
}

// ---- ICMP --------------------------------------------------------------------------------------

type icmpMonitorModel struct {
	monitorBase
	PacketSize types.Int64 `tfsdk:"packet_size"`
	Count      types.Int64 `tfsdk:"ping_count"`
}

var icmpMonitorSpec = monitorSpec{
	name:        "icmp_monitor",
	kind:        "icmp-monitors",
	title:       "ICMP monitor",
	description: "An ICMP (ping) monitor. The hostname must resolve to a public address when it is set.",
	attrs: map[string]schema.Attribute{
		"packet_size": schema.Int64Attribute{
			Optional:    true,
			Description: "Payload size in bytes, 1 to 65507. Unset = worker default.",
			Validators:  []validator.Int64{int64validator.Between(1, 65507)},
		},
		"ping_count": schema.Int64Attribute{
			Optional:    true,
			Description: "Echo requests per check, 1 to 10. Unset = worker default.",
			Validators:  []validator.Int64{int64validator.Between(1, 10)},
		},
	},
}

func (m *icmpMonitorModel) put(_ context.Context, b *bodyBuilder, _, prior *icmpMonitorModel) diag.Diagnostics {
	if !b.changed(m.PacketSize, prior.PacketSize) && !b.changed(m.Count, prior.Count) {
		return nil
	}
	// The schema is strict and rejects null; PATCH replaces icmpConfig as a whole.
	c := map[string]any{}
	if !m.PacketSize.IsNull() {
		c["packetSize"] = m.PacketSize.ValueInt64()
	}
	if !m.Count.IsNull() {
		c["count"] = m.Count.ValueInt64()
	}
	b.body["icmpConfig"] = c
	return nil
}

func (m *icmpMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	m.PacketSize = cfgInt(row.Config["packetSize"])
	m.Count = cfgInt(row.Config["count"])
	return nil
}

// ---- SMTP --------------------------------------------------------------------------------------

type smtpMonitorModel struct {
	monitorBase
	protocolPort
	Secure            types.Bool   `tfsdk:"secure"`
	IgnoreTLS         types.Bool   `tfsdk:"ignore_tls"`
	RequireTLS        types.Bool   `tfsdk:"require_tls"`
	AuthUser          types.String `tfsdk:"auth_user"`
	AuthPassWO        types.String `tfsdk:"auth_pass_wo"`
	AuthPassWOVersion types.Int64  `tfsdk:"auth_pass_wo_version"`
}

var smtpMonitorSpec = monitorSpec{
	name:        "smtp_monitor",
	kind:        "smtp-monitors",
	title:       "SMTP monitor",
	description: "An SMTP monitor: connects and authenticates (check_mode `protocol`) or only checks the port (`tcp`). The hostname must resolve to a public address.",
	attrs: merge(protocolPortAttrs("465 with `secure`, else 25"), map[string]schema.Attribute{
		"secure":               boolAttr("Implicit TLS (SMTPS). Defaults to false.", false),
		"ignore_tls":           boolAttr("Do not upgrade with STARTTLS. Defaults to false.", false),
		"require_tls":          boolAttr("Fail without STARTTLS. Defaults to false.", false),
		"auth_user":            schema.StringAttribute{Optional: true, Description: "SMTP user. Required in check_mode `protocol`."},
		"auth_pass_wo":         writeOnly("SMTP password. Required in check_mode `protocol`."),
		"auth_pass_wo_version": woVersion("`auth_pass_wo`"),
	}),
}

func (m *smtpMonitorModel) put(_ context.Context, b *bodyBuilder, cfg, prior *smtpMonitorModel) diag.Diagnostics {
	protocol, full := m.putPort(b, &prior.protocolPort)
	if !protocol {
		return nil
	}
	secret := full || b.differs(m.AuthPassWOVersion, prior.AuthPassWOVersion)
	if !secret && !b.anyDiffers(m.Secure, prior.Secure, m.IgnoreTLS, prior.IgnoreTLS,
		m.RequireTLS, prior.RequireTLS, m.AuthUser, prior.AuthUser) {
		return nil
	}
	// PATCH merges config (auth too) and keeps auth.pass unless it is sent.
	auth := map[string]any{"user": m.AuthUser.ValueStringPointer()}
	if secret {
		putSecret(b, auth, "pass", cfg.AuthPassWO)
	}
	b.body["smtpConfig"] = map[string]any{
		"secure":     m.Secure.ValueBool(),
		"ignoreTls":  m.IgnoreTLS.ValueBool(),
		"requireTls": m.RequireTLS.ValueBool(),
		"auth":       auth,
	}
	return nil
}

func (m *smtpMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	if m.readPort(row) {
		m.Secure, m.IgnoreTLS, m.RequireTLS = keepBool(m.Secure, false), keepBool(m.IgnoreTLS, false), keepBool(m.RequireTLS, false)
		return nil
	}
	m.Secure = cfgBool(row.Config["secure"], false)
	m.IgnoreTLS = cfgBool(row.Config["ignoreTls"], false)
	m.RequireTLS = cfgBool(row.Config["requireTls"], false)
	m.AuthUser = cfgString(cfgMap(row.Config["auth"])["user"])
	return nil
}

// ---- SSH ---------------------------------------------------------------------------------------

type sshMonitorModel struct {
	monitorBase
	protocolPort
	Username         types.String `tfsdk:"username"`
	PasswordWO       types.String `tfsdk:"password_wo"`
	PrivateKeyWO     types.String `tfsdk:"private_key_wo"`
	PassphraseWO     types.String `tfsdk:"passphrase_wo"`
	SecretsWOVersion types.Int64  `tfsdk:"secrets_wo_version"`
}

var sshMonitorSpec = monitorSpec{
	name:        "ssh_monitor",
	kind:        "ssh-monitors",
	title:       "SSH monitor",
	description: "An SSH monitor: logs in (check_mode `protocol`) or only checks the port (`tcp`). The hostname must resolve to a public address.",
	attrs: merge(protocolPortAttrs("22"), map[string]schema.Attribute{
		"username":           schema.StringAttribute{Optional: true, Description: "Login user. Required in check_mode `protocol`."},
		"password_wo":        writeOnly("Login password; `password_wo` or `private_key_wo` is required in check_mode `protocol`."),
		"private_key_wo":     writeOnly("Private key (OpenSSH/PEM)."),
		"passphrase_wo":      writeOnly("Passphrase of the private key."),
		"secrets_wo_version": woVersion("`password_wo`, `private_key_wo` and `passphrase_wo`"),
	}),
}

func (m *sshMonitorModel) put(_ context.Context, b *bodyBuilder, cfg, prior *sshMonitorModel) diag.Diagnostics {
	protocol, full := m.putPort(b, &prior.protocolPort)
	if !protocol {
		return nil
	}
	secret := full || b.differs(m.SecretsWOVersion, prior.SecretsWOVersion)
	if !secret && !b.differs(m.Username, prior.Username) {
		return nil
	}
	c := map[string]any{"username": m.Username.ValueStringPointer()}
	if secret {
		putSecret(b, c, "password", cfg.PasswordWO)
		putSecret(b, c, "privateKey", cfg.PrivateKeyWO)
		putSecret(b, c, "passphrase", cfg.PassphraseWO)
	}
	b.body["sshConfig"] = c
	return nil
}

func (m *sshMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	if m.readPort(row) {
		return nil
	}
	m.Username = cfgString(row.Config["username"])
	return nil
}

// ---- FTP ---------------------------------------------------------------------------------------

type ftpMonitorModel struct {
	monitorBase
	protocolPort
	User              types.String `tfsdk:"user"`
	TLSMode           types.String `tfsdk:"tls_mode"`
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
}

var ftpMonitorSpec = monitorSpec{
	name:        "ftp_monitor",
	kind:        "ftp-monitors",
	title:       "FTP monitor",
	description: "An FTP monitor: logs in (check_mode `protocol`) or only checks the port (`tcp`). The hostname must resolve to a public address.",
	attrs: merge(protocolPortAttrs("990 with tls_mode `implicit`, else 21"), map[string]schema.Attribute{
		"user": schema.StringAttribute{Optional: true, Description: "Login user. Required in check_mode `protocol`."},
		"tls_mode": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("none"),
			Description: "`none` (plain FTP), `explicit` (AUTH TLS) or `implicit` (FTPS). Defaults to `none`.",
			Validators:  []validator.String{stringvalidator.OneOf("none", "explicit", "implicit")},
		},
		"password_wo":         writeOnly("Login password. Required in check_mode `protocol`."),
		"password_wo_version": woVersion("`password_wo`"),
	}),
}

// ftpSecure maps tls_mode to ftpConfig.secure as the worker reads it (probes/ftp.ts):
// true = explicit TLS, "implicit" = implicit TLS, anything else = plain.
func ftpSecure(mode string) any {
	switch mode {
	case "explicit":
		return true
	case "implicit":
		return "implicit"
	}
	return false
}

func ftpTLSMode(secure any) string {
	switch secure {
	case true:
		return "explicit"
	case "implicit":
		return "implicit"
	}
	return "none"
}

func (m *ftpMonitorModel) put(_ context.Context, b *bodyBuilder, cfg, prior *ftpMonitorModel) diag.Diagnostics {
	protocol, full := m.putPort(b, &prior.protocolPort)
	if !protocol {
		return nil
	}
	secret := full || b.differs(m.PasswordWOVersion, prior.PasswordWOVersion)
	if !secret && !b.anyDiffers(m.User, prior.User, m.TLSMode, prior.TLSMode) {
		return nil
	}
	c := map[string]any{"user": m.User.ValueStringPointer(), "secure": ftpSecure(m.TLSMode.ValueString())}
	if secret {
		putSecret(b, c, "password", cfg.PasswordWO)
	}
	b.body["ftpConfig"] = c
	return nil
}

func (m *ftpMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	if m.readPort(row) {
		m.TLSMode = keepString(m.TLSMode, "none")
		return nil
	}
	m.User = cfgString(row.Config["user"])
	m.TLSMode = types.StringValue(ftpTLSMode(row.Config["secure"]))
	return nil
}

// ---- IMAP/POP3 ---------------------------------------------------------------------------------

type imapPopMonitorModel struct {
	monitorBase
	protocolPort
	Protocol              types.String `tfsdk:"protocol"`
	User                  types.String `tfsdk:"user"`
	TLS                   types.Bool   `tfsdk:"tls"`
	TLSRejectUnauthorized types.Bool   `tfsdk:"tls_reject_unauthorized"`
	PasswordWO            types.String `tfsdk:"password_wo"`
	PasswordWOVersion     types.Int64  `tfsdk:"password_wo_version"`
}

var imapPopMonitorSpec = monitorSpec{
	name:        "imap_pop_monitor",
	kind:        "imap-pop-monitors",
	title:       "IMAP/POP3 monitor",
	description: "An IMAP or POP3 monitor: logs in (check_mode `protocol`) or only checks the port (`tcp`, needs `port`). The hostname must resolve to a public address.",
	attrs: merge(protocolPortAttrs("993/143 for IMAP, 995/110 for POP3 (with/without `tls`); required in check_mode `tcp`"), map[string]schema.Attribute{
		"protocol": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("imap"),
			Description: "`imap` or `pop3`. Defaults to `imap`.",
			Validators:  []validator.String{stringvalidator.OneOf("imap", "pop3")},
		},
		"user":                    schema.StringAttribute{Optional: true, Description: "Login user. Required in check_mode `protocol`."},
		"tls":                     boolAttr("Implicit TLS (IMAPS/POP3S). Defaults to false.", false),
		"tls_reject_unauthorized": boolAttr("Reject invalid server certificates. Defaults to true.", true),
		"password_wo":             writeOnly("Login password. Required in check_mode `protocol`."),
		"password_wo_version":     woVersion("`password_wo`"),
	}),
}

func (m *imapPopMonitorModel) put(_ context.Context, b *bodyBuilder, cfg, prior *imapPopMonitorModel) diag.Diagnostics {
	protocol, full := m.putPort(b, &prior.protocolPort)
	if !protocol {
		return nil
	}
	secret := full || b.differs(m.PasswordWOVersion, prior.PasswordWOVersion)
	if !secret && !b.anyDiffers(m.Protocol, prior.Protocol, m.User, prior.User, m.TLS, prior.TLS,
		m.TLSRejectUnauthorized, prior.TLSRejectUnauthorized) {
		return nil
	}
	c := map[string]any{
		"protocol":   m.Protocol.ValueString(),
		"user":       m.User.ValueStringPointer(),
		"tls":        m.TLS.ValueBool(),
		"tlsOptions": map[string]any{"rejectUnauthorized": m.TLSRejectUnauthorized.ValueBool()},
	}
	if secret {
		putSecret(b, c, "password", cfg.PasswordWO)
	}
	b.body["imapPopConfig"] = c
	return nil
}

func (m *imapPopMonitorModel) read(_ context.Context, row *client.MonitorRow) diag.Diagnostics {
	if m.readPort(row) {
		m.Protocol = keepString(m.Protocol, "imap")
		m.TLS, m.TLSRejectUnauthorized = keepBool(m.TLS, false), keepBool(m.TLSRejectUnauthorized, true)
		return nil
	}
	m.Protocol = types.StringValue("imap")
	if row.Config["protocol"] == "pop3" {
		m.Protocol = types.StringValue("pop3")
	}
	m.User = cfgString(row.Config["user"])
	m.TLS = cfgBool(row.Config["tls"], false)
	m.TLSRejectUnauthorized = cfgBool(cfgMap(row.Config["tlsOptions"])["rejectUnauthorized"], true)
	return nil
}
