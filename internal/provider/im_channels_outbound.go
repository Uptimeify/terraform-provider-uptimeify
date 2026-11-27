package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure            = &imOutboundResource{}
	_ resource.ResourceWithImportState          = &imOutboundResource{}
	_ resource.ResourceWithValidateConfig       = &imOutboundResource{}
	_ datasource.DataSourceWithConfigure        = &imOutboundDataSource{}
	_ datasource.DataSourceWithConfigValidators = &imOutboundDataSource{}
)

type imOutboundResource struct{ client *client.Client }

// imOutboundModel is shared by the uptimeify_im_outbound_integration resource and data source.
type imOutboundModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	TeamID         types.Int64  `tfsdk:"team_id"`
	Type           types.String `tfsdk:"type"`
	Name           types.String `tfsdk:"name"`
	IsActive       types.Bool   `tfsdk:"is_active"`
	FilterTeamIDs  types.List   `tfsdk:"filter_team_ids"`
	FilterSeverity types.List   `tfsdk:"filter_severities"`
	EventKinds     types.List   `tfsdk:"event_kinds"`
	Email          types.String `tfsdk:"email"`
	BaseURL        types.String `tfsdk:"base_url"`
	ProjectKey     types.String `tfsdk:"project_key"`
	HasWebhookURL  types.Bool   `tfsdk:"has_webhook_url"`
	HasBearerToken types.Bool   `tfsdk:"has_bearer_token"`
	HasHeaders     types.Bool   `tfsdk:"has_headers"`
	HasAPIToken    types.Bool   `tfsdk:"has_api_token"`
	HasRoutingKey  types.Bool   `tfsdk:"has_routing_key"`
	HasAPIKey      types.Bool   `tfsdk:"has_api_key"`
	// Added with the slack_app, mattermost, googlechat and linear types.
	HasSigningSecret           types.Bool   `tfsdk:"has_signing_secret"`
	HasInboundToken            types.Bool   `tfsdk:"has_inbound_token"`
	ActionLinks                types.Bool   `tfsdk:"action_links"`
	MinimalPayload             types.Bool   `tfsdk:"minimal_payload"`
	WorkspaceID                types.Int64  `tfsdk:"workspace_id"`
	ChannelID                  types.String `tfsdk:"channel_id"`
	ReadOnly                   types.Bool   `tfsdk:"read_only"`
	ThreadCommentSync          types.Bool   `tfsdk:"thread_comment_sync"`
	IncidentChannelMinSeverity types.String `tfsdk:"incident_channel_min_severity"`
	IncidentChannelPrivate     types.Bool   `tfsdk:"incident_channel_private"`
	LinearTeamID               types.String `tfsdk:"linear_team_id"`
	CreatedAt                  types.String `tfsdk:"created_at"`
	UpdatedAt                  types.String `tfsdk:"updated_at"`
}

// imOutboundResourceModel adds the write-only secrets (config only, never in state).
type imOutboundResourceModel struct {
	imOutboundModel
	WebhookURLWO     types.String `tfsdk:"webhook_url_wo"`
	BearerTokenWO    types.String `tfsdk:"bearer_token_wo"`
	HeadersWO        types.Map    `tfsdk:"headers_wo"`
	APITokenWO       types.String `tfsdk:"api_token_wo"`
	RoutingKeyWO     types.String `tfsdk:"routing_key_wo"`
	APIKeyWO         types.String `tfsdk:"api_key_wo"`
	SigningSecretWO  types.String `tfsdk:"signing_secret_wo"`
	SecretsWOVersion types.Int64  `tfsdk:"secrets_wo_version"`
}

// outboundFieldTypes: the types each config attribute belongs to. The API keeps keys of other
// types unencrypted and returns them, so a secret on the wrong type would leak.
// Mirrors validateIntegrationConfigShape and SECRET_FIELDS of the app (outbound-access.ts,
// outbound-secrets.ts).
var chatTypes = []string{"slack", "teams", "discord", "mattermost", "googlechat"}

var outboundFieldTypes = map[string][]string{
	"webhook_url_wo":                append(slices.Clone(chatTypes), "webhook"),
	"minimal_payload":               chatTypes,
	"bearer_token_wo":               {"webhook"},
	"headers_wo":                    {"webhook"},
	"signing_secret_wo":             {"webhook"},
	"action_links":                  {"webhook"},
	"workspace_id":                  {"slack_app"},
	"channel_id":                    {"slack_app"},
	"read_only":                     {"slack_app"},
	"thread_comment_sync":           {"slack_app"},
	"incident_channel_min_severity": {"slack_app"},
	"incident_channel_private":      {"slack_app"},
	"api_token_wo":                  {"jira"},
	"email":                         {"jira"},
	"base_url":                      {"jira"},
	"project_key":                   {"jira"},
	"routing_key_wo":                {"pagerduty_v2"},
	"api_key_wo":                    {"opsgenie", "linear"},
	"linear_team_id":                {"linear"},
}

func (m *imOutboundResourceModel) configFields() map[string]attr.Value {
	f := m.plainFields()
	for k, v := range map[string]attr.Value{
		"webhook_url_wo": m.WebhookURLWO, "bearer_token_wo": m.BearerTokenWO, "headers_wo": m.HeadersWO,
		"signing_secret_wo": m.SigningSecretWO, "api_token_wo": m.APITokenWO, "routing_key_wo": m.RoutingKeyWO,
		"api_key_wo": m.APIKeyWO,
	} {
		f[k] = v
	}
	return f
}

// plainFields are the non-secret config attributes, keyed by attribute name.
func (m *imOutboundModel) plainFields() map[string]attr.Value {
	return map[string]attr.Value{
		"email": m.Email, "base_url": m.BaseURL, "project_key": m.ProjectKey, "linear_team_id": m.LinearTeamID,
		"minimal_payload": m.MinimalPayload, "action_links": m.ActionLinks, "workspace_id": m.WorkspaceID,
		"channel_id": m.ChannelID, "read_only": m.ReadOnly, "thread_comment_sync": m.ThreadCommentSync,
		"incident_channel_min_severity": m.IncidentChannelMinSeverity, "incident_channel_private": m.IncidentChannelPrivate,
	}
}

// plainAPIKeys maps plainFields names to the API config keys.
var plainAPIKeys = map[string]string{
	"email": "email", "base_url": "baseUrl", "project_key": "projectKey", "linear_team_id": "linearTeamId",
	"minimal_payload": "minimalPayload", "action_links": "actionLinks", "workspace_id": "workspaceId",
	"channel_id": "channelId", "read_only": "readOnly", "thread_comment_sync": "threadCommentSync",
	"incident_channel_min_severity": "incidentChannelMinSeverity", "incident_channel_private": "incidentChannelPrivate",
}

// isTrue: the API leaves a boolean switch out until it is set; absent = false.
func isTrue(b *bool) types.Bool { return types.BoolValue(b != nil && *b) }

func (m *imOutboundModel) fromAPI(o *client.ImOutbound) {
	m.ID = types.StringValue(strconv.FormatInt(o.ID, 10))
	m.OrganizationID = types.Int64Value(o.OrganizationID)
	m.TeamID = types.Int64PointerValue(o.TeamID)
	m.Type = types.StringValue(o.Type)
	m.Name = types.StringValue(o.Name)
	m.IsActive = types.BoolValue(o.IsActive)
	m.FilterTeamIDs = listOf(types.Int64Type, o.Filters.Teams, types.Int64Value)
	m.FilterSeverity = listOf(types.StringType, o.Filters.Severities, types.StringValue)
	m.EventKinds = listOf(types.StringType, o.Filters.EventKinds, types.StringValue)
	m.Email = types.StringPointerValue(o.Config.Email)
	m.BaseURL = types.StringPointerValue(o.Config.BaseURL)
	m.ProjectKey = types.StringPointerValue(o.Config.ProjectKey)
	m.HasWebhookURL = types.BoolValue(o.Config.HasWebhookURL)
	m.HasBearerToken = types.BoolValue(o.Config.HasBearerToken)
	m.HasHeaders = types.BoolValue(o.Config.HasHeaders)
	m.HasAPIToken = types.BoolValue(o.Config.HasAPIToken)
	m.HasRoutingKey = types.BoolValue(o.Config.HasRoutingKey)
	m.HasAPIKey = types.BoolValue(o.Config.HasAPIKey)
	m.HasSigningSecret = types.BoolValue(o.Config.HasSigningSecret)
	m.HasInboundToken = types.BoolValue(o.Config.HasInboundToken)
	m.ActionLinks = isTrue(o.Config.ActionLinks)
	m.MinimalPayload = isTrue(o.Config.MinimalPayload)
	m.WorkspaceID = types.Int64PointerValue(o.Config.WorkspaceID)
	m.ChannelID = types.StringPointerValue(o.Config.ChannelID)
	m.ReadOnly = isTrue(o.Config.ReadOnly)
	m.ThreadCommentSync = isTrue(o.Config.ThreadCommentSync)
	m.IncidentChannelMinSeverity = types.StringPointerValue(o.Config.IncidentChannelMinSeverity)
	m.IncidentChannelPrivate = isTrue(o.Config.IncidentChannelPrivate)
	m.LinearTeamID = types.StringPointerValue(o.Config.LinearTeamID)
	m.CreatedAt = types.StringValue(o.CreatedAt)
	m.UpdatedAt = types.StringValue(o.UpdatedAt)
}

// listOf builds a list value; nil = null, empty = [] (the API keeps the difference).
func listOf[T any, V attr.Value](elem attr.Type, in []T, val func(T) V) types.List {
	if in == nil {
		return types.ListNull(elem)
	}
	elems := make([]attr.Value, 0, len(in))
	for _, v := range in {
		elems = append(elems, val(v))
	}
	return types.ListValueMust(elem, elems)
}

func NewImOutboundResource() resource.Resource { return &imOutboundResource{} }

func (r *imOutboundResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_outbound_integration"
}

func (r *imOutboundResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	secret := func(d string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
			Description: d + " Write-only: sent on create and whenever `secrets_wo_version` changes."}
	}
	has := func(d string) schema.BoolAttribute { return schema.BoolAttribute{Computed: true, Description: d} }
	flag := func(d string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: d + " Default `false`."}
	}
	resp.Schema = schema.Schema{
		Description: "An Incident Management outbound integration: forwards incident events to Slack (incoming webhook " +
			"or the Uptimeify Slack app), Microsoft Teams, Discord, Mattermost, Google Chat, Jira, Linear, PagerDuty " +
			"(Events v2), Opsgenie or a generic webhook. Incident Management must be active for the organization. URLs, " +
			"tokens, keys and header values are secrets the API never returns, so they are write-only here; `has_*` " +
			"tells whether one is stored. A stored secret cannot be cleared by removing it from the configuration (the " +
			"API keeps it); replace the integration instead. `slack_app` posts through a Slack workspace that an org " +
			"admin connected in the Uptimeify UI (OAuth); Terraform cannot connect one, it only references it by " +
			"`workspace_id`. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric integration id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Optional: true, Description: "Owning team; unset = organization-wide (needs an org admin).",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"type": schema.StringAttribute{Required: true,
				Description: "`slack`, `slack_app`, `teams`, `discord`, `mattermost`, `googlechat`, `jira`, `linear`, " +
					"`pagerduty_v2`, `opsgenie` or `webhook`. Changing it replaces the integration.",
				Validators:    []validator.String{stringvalidator.OneOf(client.ImOutboundTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name": schema.StringAttribute{Required: true, Description: "1 to 200 characters, no surrounding whitespace.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 200)}},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Default `true`."},
			"filter_team_ids": schema.ListAttribute{ElementType: types.Int64Type, Optional: true,
				Description: "Only incidents of these teams (of the organization). Unset = all.",
				Validators:  []validator.List{listvalidator.ValueInt64sAre(int64validator.AtLeast(1))}},
			"filter_severities": schema.ListAttribute{ElementType: types.StringType, Optional: true,
				Description: "Only these severities, `sev1` to `sev4`. Unset = all.",
				Validators:  []validator.List{listvalidator.ValueStringsAre(sevValidator...)}},
			"event_kinds": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true,
				Description: "Events to forward: `incident_created`, `acknowledged`, `severity_changed`, `status_changed`, " +
					"`comment`, `resolved`. Unset = all, except `jira`, where the server stores `[\"incident_created\"]`. " +
					"Removing it from the configuration keeps the stored list.",
				Validators: []validator.List{listvalidator.ValueStringsAre(stringvalidator.OneOf(client.ImOutboundEventKinds...))}},
			"webhook_url_wo":  secret("`slack`/`teams`/`discord`/`mattermost`/`googlechat`/`webhook`: target URL (public http(s)), required."),
			"bearer_token_wo": secret("`webhook`: sent as `Authorization: Bearer`."),
			"headers_wo": schema.MapAttribute{ElementType: types.StringType, Optional: true, WriteOnly: true, Sensitive: true,
				Description: "`webhook`: extra request headers. Write-only: sent on create and whenever `secrets_wo_version` " +
					"changes, then replacing the stored set (unset = remove all)."},
			"api_token_wo":   secret("`jira`: API token, required."),
			"routing_key_wo": secret("`pagerduty_v2`: integration routing key, required."),
			"api_key_wo":     secret("`opsgenie`/`linear`: API key, required."),
			"signing_secret_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
				Validators: []validator.String{stringvalidator.LengthAtLeast(16)},
				Description: "`webhook`: HMAC key, at least 16 characters. Every delivery then carries `X-Webhook-Signature`, " +
					"the hex HMAC-SHA256 of the raw body. Write-only: sent on create and whenever `secrets_wo_version` changes."},
			"action_links": flag("`webhook`: the payload of an open incident carries signed `actions.resolveUrl` (and " +
				"`actions.acknowledgeUrl` until acknowledged), valid 24 hours, usable without an Uptimeify account by anyone " +
				"who can read the payload."),
			"minimal_payload": flag("`slack`/`teams`/`discord`/`mattermost`/`googlechat`: the message carries only severity, " +
				"title, event kind and link (no team, status, comment text, values or actor)."),
			"workspace_id": schema.Int64Attribute{Optional: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				Description: "`slack_app`: id of a Slack workspace connected to the organization (connected in the UI), required."},
			"channel_id": schema.StringAttribute{Optional: true,
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[CG][A-Z0-9]{2,20}$`), "must be a Slack channel id (C... or G...)")},
				Description: "`slack_app`: Slack channel id (starts with `C` or `G`), not the channel name, required."},
			"read_only": flag("`slack_app`: card without action buttons, for stakeholder channels."),
			"thread_comment_sync": flag("`slack_app`: replies of linked users in the thread of an incident card are stored " +
				"as timeline comments (needs the Events subscription of the Slack app)."),
			"incident_channel_min_severity": schema.StringAttribute{Optional: true, Validators: sevValidator,
				Description: "`slack_app`: incidents of this severity or worse (`sev1` to `sev4`) also get their own Slack " +
					"channel, archived on resolve. Unset = off."},
			"incident_channel_private": flag("`slack_app`: create that incident channel as a private channel (needs the " +
				"bot scope `groups:write`). Only takes effect with `incident_channel_min_severity`."),
			"linear_team_id": schema.StringAttribute{Optional: true,
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`), "must be a Linear team id (UUID)")},
				Description: "`linear`: id (UUID) of the Linear team issues are created in, not the team key, required."},
			"has_signing_secret": has("Whether a webhook signing secret is stored."),
			"has_inbound_token":  has("`jira`: whether the back channel from Jira is switched on (managed outside Terraform)."),
			"secrets_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend every `*_wo` value."},
			"email":              schema.StringAttribute{Optional: true, Description: "`jira`: account email, required."},
			"base_url":           schema.StringAttribute{Optional: true, Description: "`jira`: site URL, https only, required."},
			"project_key":        schema.StringAttribute{Optional: true, Description: "`jira`: project key, required."},
			"has_webhook_url":    has("Whether a webhook URL is stored."),
			"has_bearer_token":   has("Whether a bearer token is stored."),
			"has_headers":        has("Whether custom headers are stored."),
			"has_api_token":      has("Whether a Jira API token is stored."),
			"has_routing_key":    has("Whether a PagerDuty routing key is stored."),
			"has_api_key":        has("Whether an Opsgenie or Linear API key is stored."),
			"created_at":         schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at":         schema.StringAttribute{Computed: true},
		},
	}
}

func (r *imOutboundResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg imOutboundResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}
	typ := cfg.Type.ValueString()
	for name, v := range cfg.configFields() {
		if !v.IsNull() && !slices.Contains(outboundFieldTypes[name], typ) {
			resp.Diagnostics.AddAttributeError(path.Root(name), name+" not allowed",
				fmt.Sprintf("%s is not used by type %q; the API would store it unencrypted. Allowed for: %v.", name, typ, outboundFieldTypes[name]))
		}
	}
}

func (r *imOutboundResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// outboundSecrets returns the configured write-only values as API config keys.
func outboundSecrets(ctx context.Context, cfg *imOutboundResourceModel) map[string]any {
	out := map[string]any{}
	for key, v := range map[string]types.String{"webhookUrl": cfg.WebhookURLWO, "bearerToken": cfg.BearerTokenWO,
		"apiToken": cfg.APITokenWO, "routingKey": cfg.RoutingKeyWO, "apiKey": cfg.APIKeyWO, "signingSecret": cfg.SigningSecretWO} {
		if !v.IsNull() {
			out[key] = v.ValueString()
		}
	}
	if cfg.Type.ValueString() == "webhook" {
		out["headers"] = channelHeaders(ctx, cfg.HeadersWO)
	}
	return out
}

// outboundFilters is the full filters object (PATCH replaces it). eventKinds unknown = keep fallback.
func outboundFilters(ctx context.Context, m *imOutboundModel, fallback types.List) map[string]any {
	f := map[string]any{}
	if !m.FilterTeamIDs.IsNull() && !m.FilterTeamIDs.IsUnknown() {
		var ids []int64
		m.FilterTeamIDs.ElementsAs(ctx, &ids, false)
		f["teams"] = ids
	}
	if !m.FilterSeverity.IsNull() && !m.FilterSeverity.IsUnknown() {
		var sev []string
		m.FilterSeverity.ElementsAs(ctx, &sev, false)
		f["severities"] = sev
	}
	kinds := m.EventKinds
	if kinds.IsUnknown() {
		kinds = fallback
	}
	if !kinds.IsNull() && !kinds.IsUnknown() {
		var k []string
		kinds.ElementsAs(ctx, &k, false)
		f["eventKinds"] = k
	}
	return f
}

// plainOutboundConfig returns the changed non-secret config keys of the plan's type; a removed
// value is sent as null (the switches fall back to false through their default, never null).
func plainOutboundConfig(plan, prev *imOutboundModel) map[string]any {
	out := map[string]any{}
	prevFields := prev.plainFields()
	for name, v := range plan.plainFields() {
		if v.IsUnknown() || v.Equal(prevFields[name]) || !slices.Contains(outboundFieldTypes[name], plan.Type.ValueString()) {
			continue
		}
		switch t := v.(type) {
		case types.String:
			out[plainAPIKeys[name]] = t.ValueStringPointer()
		case types.Bool:
			out[plainAPIKeys[name]] = t.ValueBoolPointer()
		case types.Int64:
			out[plainAPIKeys[name]] = t.ValueInt64Pointer()
		}
	}
	return out
}

func (r *imOutboundResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg imOutboundResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	config := plainOutboundConfig(&plan.imOutboundModel, &imOutboundModel{})
	for k, v := range outboundSecrets(ctx, &cfg) {
		config[k] = v
	}
	body := map[string]any{"name": plan.Name.ValueString(), "type": plan.Type.ValueString(), "config": config,
		"filters": outboundFilters(ctx, &plan.imOutboundModel, types.ListNull(types.StringType)), "isActive": plan.IsActive.ValueBool()}
	if !plan.TeamID.IsNull() {
		body["teamId"] = plan.TeamID.ValueInt64()
	}
	o, err := r.client.CreateImOutbound(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating outbound integration failed", err.Error())
		return
	}
	plan.fromAPI(o)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imOutboundResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imOutboundResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetImOutbound(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading outbound integration failed", err.Error())
		return
	}
	state.fromAPI(o)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// outboundPatch builds the PATCH body: changed fields only; secrets when the version changes.
func outboundPatch(ctx context.Context, plan, state, cfg *imOutboundResourceModel) map[string]any {
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.TeamID.Equal(state.TeamID) {
		patch["teamId"] = plan.TeamID.ValueInt64Pointer()
	}
	if !plan.IsActive.Equal(state.IsActive) {
		patch["isActive"] = plan.IsActive.ValueBool()
	}
	if !plan.FilterTeamIDs.Equal(state.FilterTeamIDs) || !plan.FilterSeverity.Equal(state.FilterSeverity) ||
		(!plan.EventKinds.IsUnknown() && !plan.EventKinds.Equal(state.EventKinds)) {
		patch["filters"] = outboundFilters(ctx, &plan.imOutboundModel, state.EventKinds)
	}
	config := plainOutboundConfig(&plan.imOutboundModel, &state.imOutboundModel)
	if !plan.SecretsWOVersion.Equal(state.SecretsWOVersion) {
		for k, v := range outboundSecrets(ctx, cfg) {
			config[k] = v
		}
	}
	if len(config) > 0 {
		patch["config"] = config
	}
	return patch
}

func (r *imOutboundResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg imOutboundResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	var o *client.ImOutbound
	var err error
	if patch := outboundPatch(ctx, &plan, &state, &cfg); len(patch) > 0 {
		o, err = r.client.UpdateImOutbound(ctx, id, patch)
	} else {
		o, err = r.client.GetImOutbound(ctx, id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating outbound integration failed", err.Error())
		return
	}
	plan.fromAPI(o)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imOutboundResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imOutboundResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteImOutbound(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting outbound integration failed", err.Error())
	}
}

func (r *imOutboundResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// data source

type imOutboundDataSource struct{ client *client.Client }

func NewImOutboundDataSource() datasource.DataSource { return &imOutboundDataSource{} }

func (d *imOutboundDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_outbound_integration"
}

func (d *imOutboundDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&imOutboundResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema, "secrets_wo_version")
	attrs["id"] = dschema.StringAttribute{Optional: true, Computed: true, Description: "Numeric integration id."}
	attrs["name"] = dschema.StringAttribute{Optional: true, Computed: true}
	resp.Schema = dschema.Schema{
		Description: "Look up one Incident Management outbound integration by `id` or by `name` (exactly one). " +
			"Fails when no or several integrations match the name. Secrets are never returned.",
		Attributes: attrs,
	}
}

func (d *imOutboundDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *imOutboundDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imOutboundDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imOutboundModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.ImOutbound
	if !cfg.ID.IsNull() {
		o, err := d.client.GetImOutbound(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading outbound integration failed", err.Error())
			return
		}
		found = o
	} else {
		all, err := d.client.ListImOutbound(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing outbound integrations failed", err.Error())
			return
		}
		all = slices.DeleteFunc(all, func(o client.ImOutbound) bool { return o.Name != cfg.Name.ValueString() })
		if len(all) != 1 {
			resp.Diagnostics.AddError("Outbound integration lookup by name failed",
				fmt.Sprintf("%d outbound integrations are named %q, expected exactly 1. Use id instead.", len(all), cfg.Name.ValueString()))
			return
		}
		found = &all[0]
	}
	cfg.fromAPI(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
