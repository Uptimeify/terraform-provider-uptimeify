package provider

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &imAlertSourceResource{}
	_ resource.ResourceWithImportState = &imAlertSourceResource{}
	_ resource.ResourceWithModifyPlan  = &imAlertSourceResource{}
)

type imAlertSourceResource struct{ client *client.Client }

// imAlertSourceModel is shared by the uptimeify_im_alert_source resource and data source.
type imAlertSourceModel struct {
	ID                    types.String               `tfsdk:"id"`
	OrganizationID        types.Int64                `tfsdk:"organization_id"`
	TeamID                types.Int64                `tfsdk:"team_id"`
	Name                  types.String               `tfsdk:"name"`
	Type                  types.String               `tfsdk:"type"`
	PayloadMapping        types.Object               `tfsdk:"payload_mapping"`
	AutoResolve           types.Bool                 `tfsdk:"auto_resolve"`
	GroupingWindowMinutes types.Int64                `tfsdk:"grouping_window_minutes"`
	MaintenanceWindows    []imMaintenanceWindowModel `tfsdk:"maintenance_windows"`
	URLTokenEnabled       types.Bool                 `tfsdk:"url_token_enabled"`
	HMACEnabled           types.Bool                 `tfsdk:"hmac_enabled"`
	BearerEnabled         types.Bool                 `tfsdk:"bearer_enabled"`
	SecondaryExpiresAt    types.String               `tfsdk:"secondary_expires_at"`
	CreatedAt             types.String               `tfsdk:"created_at"`
	UpdatedAt             types.String               `tfsdk:"updated_at"`
}

// imAlertSourceResourceModel adds the secrets. hmac_secret_wo is write-only (config only);
// the server-minted tokens are shown once by the API and kept in state from that answer.
type imAlertSourceResourceModel struct {
	imAlertSourceModel
	HMACSecretWO        types.String `tfsdk:"hmac_secret_wo"`
	HMACSecretWOVersion types.Int64  `tfsdk:"hmac_secret_wo_version"`
	IngestToken         types.String `tfsdk:"ingest_token"`
	IngestURL           types.String `tfsdk:"ingest_url"`
	InboundEmailAddress types.String `tfsdk:"inbound_email_address"`
	IngestTokenRotation types.String `tfsdk:"ingest_token_rotation"`
	BearerToken         types.String `tfsdk:"bearer_token"`
	BearerTokenRotation types.String `tfsdk:"bearer_token_rotation"`
}

// imAlertSourceResourceOnly are the resource attributes the data source leaves out.
var imAlertSourceResourceOnly = []string{"hmac_secret_wo_version", "ingest_token", "ingest_url",
	"inbound_email_address", "ingest_token_rotation", "bearer_token", "bearer_token_rotation"}

func payloadMappingAttrTypes() map[string]attr.Type {
	return payloadMappingSchema().GetType().(types.ObjectType).AttrTypes
}

// fromAPI fills m. payload_mapping is read only when it is managed (set in m) or full is true
// (import, data source); an empty stored mapping {} written as such stays as written.
func (m *imAlertSourceModel) fromAPI(ctx context.Context, s *client.AlertSource, full bool) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(strconv.FormatInt(s.ID, 10))
	m.OrganizationID = types.Int64Value(s.OrganizationID)
	m.TeamID = types.Int64Value(s.TeamID)
	m.Name = types.StringValue(s.Name)
	m.Type = types.StringValue(s.Type)
	m.AutoResolve = types.BoolValue(s.AutoResolve)
	m.GroupingWindowMinutes = types.Int64PointerValue(s.GroupingWindowMinutes)
	m.MaintenanceWindows = maintenanceFromAPI(s.Maintenance)
	m.URLTokenEnabled = types.BoolPointerValue(s.AuthMode.URLToken)
	m.HMACEnabled = types.BoolValue(s.AuthMode.HMAC != nil && s.AuthMode.HMAC.Enabled)
	m.BearerEnabled = types.BoolValue(s.AuthMode.Bearer != nil && s.AuthMode.Bearer.Enabled)
	m.SecondaryExpiresAt = types.StringPointerValue(s.SecondaryExpiresAt)
	m.CreatedAt = types.StringValue(s.CreatedAt)
	m.UpdatedAt = types.StringValue(s.UpdatedAt)

	pm := payloadMappingFromAPI(s.PayloadMapping)
	managed := full || (!m.PayloadMapping.IsNull() && !m.PayloadMapping.IsUnknown())
	switch {
	case !managed:
		m.PayloadMapping = types.ObjectNull(payloadMappingAttrTypes())
	case pm != nil:
		m.PayloadMapping, diags = types.ObjectValueFrom(ctx, payloadMappingAttrTypes(), pm)
	case !m.PayloadMapping.IsNull() && !m.PayloadMapping.IsUnknown():
		// an empty mapping {} written as such stays as written
	default:
		m.PayloadMapping = types.ObjectNull(payloadMappingAttrTypes())
	}
	return diags
}

func NewImAlertSourceResource() resource.Resource { return &imAlertSourceResource{} }

func (r *imAlertSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_alert_source"
}

func (r *imAlertSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An Incident Management alert source (inbound integration). Incident Management must be active " +
			"for the organization. Import with the numeric id; the server-minted tokens are shown by the API only " +
			"once and are therefore null after an import until rotated.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric source id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Required: true, Description: "Team that owns incidents from this source.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"name": schema.StringAttribute{Required: true, Description: "1 to 200 characters, no surrounding whitespace.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 200)}},
			"type": schema.StringAttribute{
				Required: true,
				Description: "A preset name from `GET /api/im/sources/presets` (e.g. `zabbix`, `datadog`, `grafana`, " +
					"`alertmanager`, `sentry`, `beszel`, `custom`), `api` (no webhook URL, fed by `POST /api/im/events`) " +
					"or `email`. Changing it replaces the source.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"payload_mapping": payloadMappingSchema(),
			"auto_resolve": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Resolve the incident when the vendor sends a resolve. Default `true`."},
			"grouping_window_minutes": schema.Int64Attribute{Optional: true,
				Description: "Group alerts arriving within this many minutes into one incident. Unset = group by dedup key only.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)}},
			"maintenance_windows": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Recurring windows in which alerts are stored as suppressed instead of opening incidents. At most 20.",
				Validators:  []validator.List{listvalidator.SizeAtMost(20)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"days_of_week": schema.ListAttribute{ElementType: types.Int64Type, Optional: true,
						Description: "ISO weekdays, 1 = Monday to 7 = Sunday. Unset = every day.",
						Validators:  []validator.List{listvalidator.ValueInt64sAre(int64validator.Between(1, 7))}},
					"from": schema.StringAttribute{Required: true, Description: "`HH:MM`, 24 h.", Validators: []validator.String{srcHHMM}},
					"to":   schema.StringAttribute{Required: true, Description: "`HH:MM`, 24 h.", Validators: []validator.String{srcHHMM}},
					"timezone": schema.StringAttribute{Optional: true,
						Description: "IANA zone. Unset = the organization zone."},
				}},
			},
			"url_token_enabled": schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB,
				Description: "`authMode.urlToken`. The server stores the flag; unset = not sent."},
			"hmac_enabled": schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB,
				Description: "Require an HMAC signature. Enabling needs a secret: set `hmac_secret_wo` (once is enough, " +
					"disabling keeps the secret stored)."},
			"hmac_secret_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Write-only HMAC secret, at least 16 characters. Sent with create and whenever " +
					"`hmac_secret_wo_version` changes; it also enables HMAC. A new secret replaces the old one at once (no grace period).",
				Validators: []validator.String{stringvalidator.LengthAtLeast(16)}},
			"hmac_secret_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `hmac_secret_wo`."},
			"bearer_enabled": schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB,
				Description: "Require `Authorization: Bearer <bearer_token>`. Enabling it the first time mints the token."},
			"bearer_token": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keep,
				Description: "Bearer token, minted by the server when bearer auth is first enabled or rotated. The API shows " +
					"it only in that answer: null after an import until `bearer_token_rotation` changes."},
			"bearer_token_rotation": schema.StringAttribute{Optional: true,
				Description: "Any value; changing it mints a new bearer token (the old one stops working at once) and enables bearer auth."},
			"ingest_token": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keep,
				Description: "Ingest token of the webhook URL (null for `type = \"api\"`). The API shows it only on create " +
					"and rotation: null after an import until `ingest_token_rotation` changes."},
			"ingest_url": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keep,
				Description: "Webhook URL `<endpoint>/api/im/ingest/<ingest_token>` for preset sources."},
			"inbound_email_address": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keep,
				Description: "`type = \"email\"`: address to forward vendor alert emails to (contains the ingest token)."},
			"ingest_token_rotation": schema.StringAttribute{Optional: true,
				Description: "Any value; changing it rotates the ingest token. The old token keeps working until `secondary_expires_at`."},
			"secondary_expires_at": schema.StringAttribute{Computed: true, PlanModifiers: keep,
				Description: "End of the grace period of the previous ingest token after a rotation."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

var srcHHMM = stringvalidator.RegexMatches(regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`), "must be HH:MM (24 h), e.g. 18:00")

func (r *imAlertSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// mintsBearer: a rotation trigger changed to a value, or bearer auth is switched on without a token in state.
func mintsBearer(plan, state *imAlertSourceResourceModel) bool {
	if state == nil {
		return !plan.BearerTokenRotation.IsNull() || plan.BearerEnabled.ValueBool()
	}
	if !plan.BearerTokenRotation.Equal(state.BearerTokenRotation) && !plan.BearerTokenRotation.IsNull() {
		return true
	}
	return plan.BearerEnabled.ValueBool() && !state.BearerEnabled.ValueBool() && state.BearerToken.IsNull()
}

func setsHMAC(plan, state *imAlertSourceResourceModel, cfg *imAlertSourceResourceModel) bool {
	if cfg.HMACSecretWO.IsNull() {
		return false
	}
	return state == nil || !plan.HMACSecretWOVersion.Equal(state.HMACSecretWOVersion)
}

func rotatesIngest(plan, state *imAlertSourceResourceModel) bool {
	return state != nil && !plan.IngestTokenRotation.Equal(state.IngestTokenRotation) && !plan.IngestTokenRotation.IsNull()
}

func (r *imAlertSourceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state, cfg imAlertSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rotatesIngest(&plan, &state) {
		plan.IngestToken = types.StringUnknown()
		plan.IngestURL = types.StringUnknown()
		plan.InboundEmailAddress = types.StringUnknown()
		plan.SecondaryExpiresAt = types.StringUnknown()
	}
	if mintsBearer(&plan, &state) {
		plan.BearerToken = types.StringUnknown()
		if cfg.BearerEnabled.IsNull() {
			plan.BearerEnabled = types.BoolUnknown()
		}
	}
	if setsHMAC(&plan, &state, &cfg) && cfg.HMACEnabled.IsNull() {
		plan.HMACEnabled = types.BoolUnknown()
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *imAlertSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg imAlertSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{
		"name":   plan.Name.ValueString(),
		"teamId": plan.TeamID.ValueInt64(),
		"type":   plan.Type.ValueString(),
	}
	if !plan.PayloadMapping.IsNull() && !plan.PayloadMapping.IsUnknown() {
		if plan.Type.ValueString() == "email" {
			resp.Diagnostics.AddAttributeError(path.Root("payload_mapping"), "payload_mapping not allowed",
				"Leave payload_mapping unset for type = \"email\": the server builds that mapping.")
			return
		}
		pm, d := mappingFromObject(ctx, plan.PayloadMapping)
		if resp.Diagnostics.Append(d...); d.HasError() {
			return
		}
		body["payloadMapping"] = pm
	}
	if !plan.URLTokenEnabled.IsNull() && !plan.URLTokenEnabled.IsUnknown() {
		body["authMode"] = map[string]any{"urlToken": plan.URLTokenEnabled.ValueBool()}
	}
	if !plan.AutoResolve.IsUnknown() {
		body["autoResolve"] = plan.AutoResolve.ValueBool()
	}
	if !plan.GroupingWindowMinutes.IsNull() {
		body["groupingWindowMinutes"] = plan.GroupingWindowMinutes.ValueInt64()
	}
	src, err := r.client.CreateAlertSource(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating alert source failed", err.Error())
		return
	}
	id := strconv.FormatInt(src.ID, 10)
	got := plan
	got.IngestToken = types.StringPointerValue(src.IngestToken)
	got.InboundEmailAddress = types.StringPointerValue(src.InboundEmailAddress)
	got.IngestURL = r.ingestURL(src.Type, src.IngestToken)
	got.BearerToken = types.StringNull()

	if plan.MaintenanceWindows != nil {
		if src, err = r.client.UpdateAlertSource(ctx, id, map[string]any{"maintenance": maintenanceToAPI(plan.MaintenanceWindows)}); err != nil {
			resp.Diagnostics.AddError("Setting alert source maintenance failed", err.Error())
		}
	}
	if !resp.Diagnostics.HasError() {
		r.converge(ctx, id, &plan, nil, &cfg, src, &got, &resp.Diagnostics)
	}
	// Stored even after an error, so the created source is tracked (tainted) and not orphaned.
	got.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &got)...)
}

func (r *imAlertSourceResource) ingestURL(typ string, token *string) types.String {
	if token == nil || typ == "email" {
		return types.StringNull()
	}
	return types.StringValue(r.client.Endpoint() + "/api/im/ingest/" + *token)
}

// converge runs the secret endpoints and the authMode switches, then reads the source into got.
// cur is the latest API answer.
func (r *imAlertSourceResource) converge(ctx context.Context, id string, plan, state, cfg *imAlertSourceResourceModel,
	cur *client.AlertSource, got *imAlertSourceResourceModel, diags *diag.Diagnostics) {
	if rotatesIngest(plan, state) {
		token, err := r.client.RotateIngestToken(ctx, id)
		if err != nil {
			diags.AddError("Rotating ingest token failed", err.Error())
			return
		}
		got.IngestToken = types.StringValue(token)
		got.IngestURL = r.ingestURL(plan.Type.ValueString(), &token)
		got.InboundEmailAddress = types.StringNull()
		if _, domain, ok := strings.Cut(state.InboundEmailAddress.ValueString(), "@"); ok {
			got.InboundEmailAddress = types.StringValue(token + "@" + domain)
		}
	}
	if setsHMAC(plan, state, cfg) {
		if err := r.client.SetHMACSecret(ctx, id, cfg.HMACSecretWO.ValueString()); err != nil {
			diags.AddError("Setting HMAC secret failed", err.Error())
			return
		}
		cur.AuthMode.HMAC = &client.AuthToggle{Enabled: true}
	}
	if mintsBearer(plan, state) {
		token, err := r.client.RotateBearerToken(ctx, id)
		if err != nil {
			diags.AddError("Minting bearer token failed", err.Error())
			return
		}
		got.BearerToken = types.StringValue(token)
		cur.AuthMode.Bearer = &client.AuthToggle{Enabled: true}
	}

	auth := map[string]any{}
	if v := plan.URLTokenEnabled; !v.IsNull() && !v.IsUnknown() && (cur.AuthMode.URLToken == nil || *cur.AuthMode.URLToken != v.ValueBool()) {
		auth["urlToken"] = v.ValueBool()
	}
	if v := plan.HMACEnabled; !v.IsUnknown() && v.ValueBool() != (cur.AuthMode.HMAC != nil && cur.AuthMode.HMAC.Enabled) {
		auth["hmac"] = map[string]any{"enabled": v.ValueBool()}
	}
	if v := plan.BearerEnabled; !v.IsUnknown() && v.ValueBool() != (cur.AuthMode.Bearer != nil && cur.AuthMode.Bearer.Enabled) {
		auth["bearer"] = map[string]any{"enabled": v.ValueBool()}
	}
	if len(auth) > 0 {
		if _, err := r.client.UpdateAlertSource(ctx, id, map[string]any{"authMode": auth}); err != nil {
			diags.AddError("Updating alert source auth failed", err.Error())
			return
		}
	}

	src, err := r.client.GetAlertSource(ctx, id)
	if err != nil {
		diags.AddError("Reading alert source failed", err.Error())
		return
	}
	diags.Append(got.fromAPI(ctx, src, false)...)
}

func mappingFromObject(ctx context.Context, o types.Object) (client.PayloadMapping, diag.Diagnostics) {
	var pm payloadMappingModel
	d := o.As(ctx, &pm, basetypes.ObjectAsOptions{})
	return pm.toAPI(), d
}

func (r *imAlertSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imAlertSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	src, err := r.client.GetAlertSource(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading alert source failed", err.Error())
		return
	}
	imported, d := req.Private.GetKey(ctx, importedKey)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(state.fromAPI(ctx, src, imported != nil)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if imported != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, nil)...)
	}
}

func (r *imAlertSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg imAlertSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.TeamID.Equal(state.TeamID) {
		patch["teamId"] = plan.TeamID.ValueInt64()
	}
	if !plan.PayloadMapping.IsUnknown() && !plan.PayloadMapping.IsNull() && !plan.PayloadMapping.Equal(state.PayloadMapping) {
		pm, d := mappingFromObject(ctx, plan.PayloadMapping)
		if resp.Diagnostics.Append(d...); d.HasError() {
			return
		}
		patch["payloadMapping"] = pm
	}
	if !plan.AutoResolve.Equal(state.AutoResolve) {
		patch["autoResolve"] = plan.AutoResolve.ValueBool()
	}
	if !plan.GroupingWindowMinutes.Equal(state.GroupingWindowMinutes) {
		patch["groupingWindowMinutes"] = plan.GroupingWindowMinutes.ValueInt64Pointer()
	}
	if !reflect.DeepEqual(plan.MaintenanceWindows, state.MaintenanceWindows) {
		patch["maintenance"] = maintenanceToAPI(plan.MaintenanceWindows) // nil = null clears
	}
	var cur *client.AlertSource
	var err error
	if len(patch) > 0 {
		cur, err = r.client.UpdateAlertSource(ctx, id, patch)
	} else {
		cur, err = r.client.GetAlertSource(ctx, id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating alert source failed", err.Error())
		return
	}
	got := plan
	got.IngestToken, got.IngestURL, got.InboundEmailAddress = state.IngestToken, state.IngestURL, state.InboundEmailAddress
	got.BearerToken = state.BearerToken
	r.converge(ctx, id, &plan, &state, &cfg, cur, &got, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &got)...)
}

func (r *imAlertSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imAlertSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAlertSource(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting alert source failed", err.Error())
	}
}

// importedKey marks the first Read after an import, which reads the stored payload mapping in full.
const importedKey = "imported"

func (r *imAlertSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, []byte(`true`))...)
}
