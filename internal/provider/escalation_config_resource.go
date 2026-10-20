package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.ResourceWithConfigure   = &escalationConfigResource{}
	_ resource.ResourceWithImportState = &escalationConfigResource{}
)

type escalationConfigResource struct{ client *client.Client }

type escalationConfigModel struct {
	ID                      types.String `tfsdk:"id"`
	OrganizationID          types.Int64  `tfsdk:"organization_id"`
	WebhookURL              types.String `tfsdk:"webhook_url"`
	WebhookMethod           types.String `tfsdk:"webhook_method"`
	WebhookBodyTemplate     types.String `tfsdk:"webhook_body_template"`
	WebhookTimeout          types.Int64  `tfsdk:"webhook_timeout"`
	WebhookRetryAttempts    types.Int64  `tfsdk:"webhook_retry_attempts"`
	WebhookRetryDelay       types.Int64  `tfsdk:"webhook_retry_delay"`
	ExpectedStatusCodes     types.String `tfsdk:"expected_status_codes"`
	IsActive                types.Bool   `tfsdk:"is_active"`
	WebhookHeadersWO        types.Map    `tfsdk:"webhook_headers_wo"`
	WebhookHeadersWOVersion types.Int64  `tfsdk:"webhook_headers_wo_version"`
}

func (m *escalationConfigModel) fromAPI(e *client.EscalationConfig) {
	m.ID = types.StringValue(strconv.FormatInt(e.OrganizationID, 10))
	m.OrganizationID = types.Int64Value(e.OrganizationID)
	m.WebhookURL = nonEmpty(e.WebhookURL)
	m.WebhookMethod = types.StringPointerValue(e.WebhookMethod)
	m.WebhookBodyTemplate = types.StringPointerValue(e.WebhookBodyTemplate)
	m.WebhookTimeout = types.Int64PointerValue(e.WebhookTimeout)
	m.WebhookRetryAttempts = types.Int64PointerValue(e.WebhookRetryAttempts)
	m.WebhookRetryDelay = types.Int64PointerValue(e.WebhookRetryDelay)
	m.ExpectedStatusCodes = types.StringPointerValue(e.ExpectedStatusCodes)
	m.IsActive = types.BoolValue(e.IsActive)
}

func NewEscalationConfigResource() resource.Resource { return &escalationConfigResource{} }

func (r *escalationConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_config"
}

func (r *escalationConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "The organization's escalation webhook (one per organization; organization token with write access). " +
			"Creating it adopts the stored configuration and applies the configured values. The webhook URL also " +
			"drives the organization webhook channel (`Organization Webhook`), so do not manage that channel as " +
			"`uptimeify_notification_channel` too. Destroying it sets `is_active = false` and clears the URL, which " +
			"deactivates the organization-level webhook channels; the row stays. Import with the organization id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "The organization id."},
			"organization_id": schema.Int64Attribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown(), int64planmodifier.RequiresReplace()},
				Description:   "Unset = the token's organization.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"webhook_url": schema.StringAttribute{Optional: true,
				Description: "Target URL (http/https, must resolve to a public address). Give it normalized (as `new URL()` prints it, e.g. with the trailing `/` of a bare host). Unset clears it and switches the organization webhook channel off."},
			"webhook_method": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "`POST`, `PUT` or `PATCH`. Unset = keep (default `POST`).", Validators: []validator.String{stringvalidator.OneOf("POST", "PUT", "PATCH")}},
			"webhook_body_template": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "JSON template with placeholders such as `{{websiteName}}`. Unset = keep."},
			"webhook_timeout": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Seconds, 1 to 30. Unset = keep (default 30).", Validators: []validator.Int64{int64validator.Between(1, 30)}},
			"webhook_retry_attempts": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "0 to 10. Unset = keep (default 3).", Validators: []validator.Int64{int64validator.Between(0, 10)}},
			"webhook_retry_delay": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Seconds, 0 to 60. Unset = keep (default 60).", Validators: []validator.Int64{int64validator.Between(0, 60)}},
			"expected_status_codes": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "Comma-separated, e.g. `200,201,202,204`. Unset = keep.",
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^\d{3}(,\d{3})*$`), "must be comma-separated status codes")}},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"webhook_headers_wo": schema.MapAttribute{Optional: true, WriteOnly: true, Sensitive: true, ElementType: types.StringType,
				Description: "Write-only request headers (they may carry tokens). Sent with create and whenever " +
					"`webhook_headers_wo_version` changes; replaces all stored headers. Authorization, Cookie, Host, " +
					"X-Forwarded-* and Proxy-* are refused by the API.",
				Validators: []validator.Map{mapvalidator.SizeAtMost(50)}},
			"webhook_headers_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `webhook_headers_wo`."},
		},
	}
}

func (r *escalationConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func headersBody(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	out := map[string]string{}
	d := m.ElementsAs(ctx, &out, false)
	return out, d
}

// patch builds the PATCH body; state nil = create (send every known value).
func (r *escalationConfigResource) patch(ctx context.Context, plan, cfg *escalationConfigModel, state *escalationConfigModel, diags *diag.Diagnostics) map[string]any {
	p := map[string]any{}
	var prev escalationConfigModel
	if state != nil {
		prev = *state
	}
	setS := func(key string, v, old types.String) {
		if !v.IsUnknown() && (state == nil && !v.IsNull() || state != nil && !v.Equal(old)) {
			p[key] = strOrNil(v)
		}
	}
	setI := func(key string, v, old types.Int64) {
		if !v.IsUnknown() && !v.IsNull() && (state == nil || !v.Equal(old)) {
			p[key] = v.ValueInt64()
		}
	}
	setS("webhookUrl", plan.WebhookURL, prev.WebhookURL)
	setS("webhookMethod", plan.WebhookMethod, prev.WebhookMethod)
	setS("webhookBodyTemplate", plan.WebhookBodyTemplate, prev.WebhookBodyTemplate)
	setS("expectedStatusCodes", plan.ExpectedStatusCodes, prev.ExpectedStatusCodes)
	setI("webhookTimeout", plan.WebhookTimeout, prev.WebhookTimeout)
	setI("webhookRetryAttempts", plan.WebhookRetryAttempts, prev.WebhookRetryAttempts)
	setI("webhookRetryDelay", plan.WebhookRetryDelay, prev.WebhookRetryDelay)
	_, urlSent := p["webhookUrl"]
	if state == nil || urlSent || !plan.IsActive.Equal(prev.IsActive) {
		// With a URL the API also writes isActive onto the organization webhook channel.
		p["isActive"] = plan.IsActive.ValueBool()
	}
	if !cfg.WebhookHeadersWO.IsNull() && (state == nil || !plan.WebhookHeadersWOVersion.Equal(prev.WebhookHeadersWOVersion)) {
		h, d := headersBody(ctx, cfg.WebhookHeadersWO)
		diags.Append(d...)
		p["webhookHeaders"] = h
	}
	return p
}

func (r *escalationConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg escalationConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID := plan.OrganizationID.ValueInt64()
	if plan.OrganizationID.IsUnknown() {
		id, err := r.client.OwnOrganizationID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Reading the token's organization failed", err.Error())
			return
		}
		orgID = id
	}
	body := r.patch(ctx, &plan, &cfg, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client.UpdateEscalationConfig(ctx, orgID, body)
	if err != nil {
		resp.Diagnostics.AddError("Saving escalation config failed", err.Error())
		return
	}
	plan.fromAPI(e)
	plan.WebhookHeadersWO = types.MapNull(types.StringType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *escalationConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client.GetEscalationConfig(ctx, state.OrganizationID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading escalation config failed", err.Error())
		return
	}
	if e.ID == nil { // nothing saved (GET answers the defaults without a row)
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromAPI(e)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *escalationConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg escalationConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.patch(ctx, &plan, &cfg, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client.UpdateEscalationConfig(ctx, state.OrganizationID.ValueInt64(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating escalation config failed", err.Error())
		return
	}
	plan.fromAPI(e)
	plan.WebhookHeadersWO = types.MapNull(types.StringType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete: there is no DELETE route; switch the webhook off instead.
func (r *escalationConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"isActive": false}
	if !state.WebhookURL.IsNull() {
		body["webhookUrl"] = nil
	}
	if _, err := r.client.UpdateEscalationConfig(ctx, state.OrganizationID.ValueInt64(), body); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Switching escalation config off failed", err.Error())
	}
}

func (r *escalationConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id < 1 {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected the numeric organization id, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), id)...)
}
