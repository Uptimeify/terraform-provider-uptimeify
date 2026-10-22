package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure        = &customerResource{}
	_ resource.ResourceWithImportState      = &customerResource{}
	_ resource.ResourceWithModifyPlan       = &customerResource{}
	_ resource.ResourceWithConfigValidators = &customerResource{}
)

type customerResource struct{ client *client.Client }

type customerResourceModel struct {
	ID                          types.String `tfsdk:"id"`
	PublicID                    types.String `tfsdk:"public_id"`
	OrganizationID              types.Int64  `tfsdk:"organization_id"`
	Name                        types.String `tfsdk:"name"`
	Email                       types.String `tfsdk:"email"`
	Status                      types.String `tfsdk:"status"`
	PackageID                   types.Int64  `tfsdk:"package_id"`
	PackageType                 types.String `tfsdk:"package_type"`
	PackageDisplayName          types.String `tfsdk:"package_display_name"`
	MonthlyReportsEnabled       types.Bool   `tfsdk:"monthly_reports_enabled"`
	EmailLanguage               types.String `tfsdk:"email_language"`
	CompanyName                 types.String `tfsdk:"company_name"`
	NotificationEmail           types.String `tfsdk:"notification_email"`
	NotificationPhoneNumber     types.String `tfsdk:"notification_phone_number"`
	CustomFields                types.String `tfsdk:"custom_fields"`
	NotificationTargets         types.Map    `tfsdk:"notification_targets"`
	AllowSelfService            types.Bool   `tfsdk:"allow_self_service"`
	MaxSelfServiceURLs          types.Int64  `tfsdk:"max_self_service_urls"`
	CanEditManaged              types.Bool   `tfsdk:"can_edit_managed"`
	EnableEmailAlerts           types.Bool   `tfsdk:"enable_email_alerts"`
	EnableSmsAlerts             types.Bool   `tfsdk:"enable_sms_alerts"`
	EnableWebhookAlerts         types.Bool   `tfsdk:"enable_webhook_alerts"`
	EnableIntegrationAlerts     types.Bool   `tfsdk:"enable_integration_alerts"`
	EnablePostRequestEscalation types.Bool   `tfsdk:"enable_post_request_escalation"`
	DeleteUserAction            types.String `tfsdk:"delete_user_action"`
	CreatedAt                   types.String `tfsdk:"created_at"`
	UpdatedAt                   types.String `tfsdk:"updated_at"`
}

// fromAPI sets everything GET returns. delete_user_action stays as it is, and so do the overrides
// when GET leaves them out (callers that may not write them).
func (m *customerResourceModel) fromAPI(ctx context.Context, c *client.CustomerDetail) diag.Diagnostics {
	// package_type keeps the identifier as written (key or display name) while the package stays
	// the same; after an import or a package change elsewhere it takes the API's key.
	if m.PackageType.IsNull() || m.PackageType.IsUnknown() ||
		!m.PackageID.IsUnknown() && !m.PackageID.Equal(types.Int64PointerValue(c.PackageID)) {
		m.PackageType = types.StringValue(c.PackageType)
	}
	m.ID = types.StringValue(strconv.FormatInt(c.ID, 10))
	m.PublicID = types.StringValue(c.PublicID)
	m.OrganizationID = types.Int64Value(c.OrganizationID)
	m.Name = types.StringValue(c.Name)
	m.Email = nonEmpty(c.Email)
	m.Status = types.StringValue(c.Status)
	m.PackageID = types.Int64PointerValue(c.PackageID)
	m.PackageDisplayName = types.StringPointerValue(c.PackageDisplayName)
	m.MonthlyReportsEnabled = types.BoolValue(c.MonthlyReportsEnabled)
	m.EmailLanguage = types.StringPointerValue(c.EmailLanguage)
	m.CompanyName = nonEmpty(c.CompanyName)
	m.NotificationEmail = nonEmpty(c.NotificationEmail)
	m.NotificationPhoneNumber = nonEmpty(c.NotificationPhoneNumber)
	cf := map[string]any{}
	if len(c.CustomFields) > 0 && string(c.CustomFields) != "null" {
		_ = json.Unmarshal(c.CustomFields, &cf)
	}
	m.CustomFields = jsonKeep(m.CustomFields, cf)
	var diags diag.Diagnostics
	if c.NotificationTargets == nil {
		m.NotificationTargets = types.MapNull(types.StringType)
	} else {
		m.NotificationTargets, diags = types.MapValueFrom(ctx, types.StringType, c.NotificationTargets)
	}
	for _, o := range []struct {
		dst *types.Bool
		got client.Opt[bool]
	}{
		{&m.AllowSelfService, c.AllowSelfService}, {&m.CanEditManaged, c.CanEditManaged},
		{&m.EnableEmailAlerts, c.EnableEmailAlerts}, {&m.EnableSmsAlerts, c.EnableSmsAlerts},
		{&m.EnableWebhookAlerts, c.EnableWebhookAlerts}, {&m.EnableIntegrationAlerts, c.EnableIntegrationAlerts},
		{&m.EnablePostRequestEscalation, c.EnablePostRequestEscalation},
	} {
		if o.got.Set {
			*o.dst = types.BoolPointerValue(o.got.V)
		}
	}
	if c.MaxSelfServiceURLs.Set {
		m.MaxSelfServiceURLs = types.Int64PointerValue(c.MaxSelfServiceURLs.V)
	}
	m.CreatedAt = types.StringValue(c.CreatedAt)
	m.UpdatedAt = types.StringValue(c.UpdatedAt)
	return diags
}

func NewCustomerResource() resource.Resource { return &customerResource{} }

func (r *customerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer"
}

func (r *customerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	optStr := func(desc string, v ...validator.String) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Description: desc, Validators: append(v, stringvalidator.LengthAtMost(255))}
	}
	override := func(what string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Description: what + " Unset = package default. " + overrideNote}
	}
	resp.Schema = schema.Schema{
		Description: "A customer of the organization. Needs an organization token with write access (customer tokens " +
			"cannot create customers). Destroying it deletes the customer with its monitors and history. " +
			"Import with the numeric id or the public id. `delete_user_action` cannot be read back; the permission " +
			"overrides are read back only with a token that may write them, otherwise they stay unset after an import.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric customer id. Use `tonumber(id)` for `customer_id` arguments."},
			"public_id":       schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"name":            schema.StringAttribute{Required: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(255)}},
			"email":           optStr("Contact email of the customer.", trimmedText),
			"status": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("active"),
				Description: "`active` or `inactive`; `inactive` pauses every monitor of the customer.",
				Validators:  []validator.String{stringvalidator.OneOf("active", "inactive")}},
			"package_id": schema.Int64Attribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Description:   "Package configuration id of the organization. Set this or `package_type`. Changing the package needs organization write access.",
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"package_type": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: "Package key or display name, resolved to `package_id`. Set this or `package_id`. Kept as written " +
					"while the package stays the same; otherwise (import, unset, package changed elsewhere) the API's package key.",
				Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(64)}},
			"package_display_name": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"monthly_reports_enabled": schema.BoolAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Description:   "Monthly report mails. Unset at create = package default; unset later = keep."},
			"email_language":            optStr("Language of mails to this customer. Unset = organization language."),
			"company_name":              optStr("", trimmedText),
			"notification_email":        optStr("Alert recipient address of this customer.", trimmedText),
			"notification_phone_number": optStr("Alert SMS number of this customer.", trimmedText),
			"custom_fields": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: "Values of the organization's custom fields as a JSON object (`jsonencode({ contract = \"A-1\" })`), keyed by `field_key`. Replaced as a whole.",
				Validators:  []validator.String{jsonObject{}}},
			"notification_targets": schema.MapAttribute{Optional: true, ElementType: types.StringType,
				Description: "Who receives each channel type: keys `sms`, `email`, `webhook`, `integrations`; values `customer` or `organization`. Unset = organization default.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.OneOf("sms", "email", "webhook", "integrations")),
					mapvalidator.ValueStringsAre(stringvalidator.OneOf("customer", "organization")),
				}},
			"allow_self_service": override("Customer logins may add their own monitors."),
			"max_self_service_urls": schema.Int64Attribute{Optional: true, Validators: []validator.Int64{int64validator.AtLeast(0)},
				Description: "Cap for self-service monitors. Unset = package default. " + overrideNote},
			"can_edit_managed": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Customer logins may edit managed monitors. " + overrideNote},
			"enable_email_alerts":            override("Email alerts."),
			"enable_sms_alerts":              override("SMS alerts."),
			"enable_webhook_alerts":          override("Webhook alerts."),
			"enable_integration_alerts":      override("Integration alerts (Slack, PagerDuty, ...)."),
			"enable_post_request_escalation": override("POST request escalation."),
			"delete_user_action": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("deactivate"),
				Description: "On destroy, logins assigned only to this customer are `deactivate`d (default) or `delete`d.",
				Validators:  []validator.String{stringvalidator.OneOf("deactivate", "delete")}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

const overrideNote = "Only an organization admin token with unrestricted customer scope sets and reads it (otherwise the " +
	"API ignores it and leaves it out of the answer, so changes made outside Terraform are not detected)."

func (r *customerResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{resourcevalidator.ExactlyOneOf(path.MatchRoot("package_id"), path.MatchRoot("package_type"))}
}

func (r *customerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// ModifyPlan: a new package_type resolves to a package_id only the API knows, a new package_id to
// a package key only the API knows.
func (r *customerResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state customerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	typeChanged := plan.packageTypeChanged(&state)
	idChanged := !plan.PackageID.IsUnknown() && !plan.PackageID.Equal(state.PackageID)
	if typeChanged {
		plan.PackageID = types.Int64Unknown()
	}
	if idChanged {
		plan.PackageType = types.StringUnknown()
	}
	if typeChanged || idChanged {
		plan.PackageDisplayName = types.StringUnknown()
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// packageTypeChanged: package_type is configured with a new value (unset, the plan carries the
// state's value or unknown).
func (m *customerResourceModel) packageTypeChanged(state *customerResourceModel) bool {
	return !m.PackageType.IsNull() && !m.PackageType.IsUnknown() && !m.PackageType.Equal(state.PackageType)
}

// overrideBody adds the overrides: at create the set ones, at update the changed ones (null = inherit).
func (m *customerResourceModel) overrideBody(body map[string]any, state *customerResourceModel) {
	type pair struct {
		key        string
		plan, prev types.Bool
	}
	var prev customerResourceModel
	if state != nil {
		prev = *state
	}
	for _, p := range []pair{
		{"allowSelfService", m.AllowSelfService, prev.AllowSelfService},
		{"enableEmailAlerts", m.EnableEmailAlerts, prev.EnableEmailAlerts},
		{"enableSmsAlerts", m.EnableSmsAlerts, prev.EnableSmsAlerts},
		{"enableWebhookAlerts", m.EnableWebhookAlerts, prev.EnableWebhookAlerts},
		{"enableIntegrationAlerts", m.EnableIntegrationAlerts, prev.EnableIntegrationAlerts},
		{"enablePostRequestEscalation", m.EnablePostRequestEscalation, prev.EnablePostRequestEscalation},
	} {
		if state == nil && !p.plan.IsNull() || state != nil && !p.plan.Equal(p.prev) {
			body[p.key] = p.plan.ValueBoolPointer()
		}
	}
	if state == nil && !m.MaxSelfServiceURLs.IsNull() || state != nil && !m.MaxSelfServiceURLs.Equal(prev.MaxSelfServiceURLs) {
		body["maxSelfServiceUrls"] = m.MaxSelfServiceURLs.ValueInt64Pointer()
	}
	if state == nil || !m.CanEditManaged.Equal(prev.CanEditManaged) {
		body["canEditManaged"] = m.CanEditManaged.ValueBool()
	}
}

// overridesApplied compares the POST answer with the plan: the API drops overrides silently
// for callers without organization write access.
func (m *customerResourceModel) overridesApplied(c *client.CustomerDetail) bool {
	eq := func(p types.Bool, got client.Opt[bool]) bool {
		return p.IsNull() && got.V == nil || got.V != nil && p.ValueBool() == *got.V
	}
	return eq(m.AllowSelfService, c.AllowSelfService) && eq(m.CanEditManaged, c.CanEditManaged) &&
		eq(m.EnableEmailAlerts, c.EnableEmailAlerts) && eq(m.EnableSmsAlerts, c.EnableSmsAlerts) &&
		eq(m.EnableWebhookAlerts, c.EnableWebhookAlerts) && eq(m.EnableIntegrationAlerts, c.EnableIntegrationAlerts) &&
		eq(m.EnablePostRequestEscalation, c.EnablePostRequestEscalation) &&
		(m.MaxSelfServiceURLs.IsNull() && c.MaxSelfServiceURLs.V == nil ||
			c.MaxSelfServiceURLs.V != nil && m.MaxSelfServiceURLs.ValueInt64() == *c.MaxSelfServiceURLs.V)
}

func targetsBody(ctx context.Context, m types.Map) (any, diag.Diagnostics) {
	if m.IsNull() {
		return nil, nil
	}
	out := map[string]string{}
	d := m.ElementsAs(ctx, &out, false)
	return out, d
}

func (r *customerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": plan.Name.ValueString(), "status": plan.Status.ValueString()}
	if !plan.PackageType.IsNull() && !plan.PackageType.IsUnknown() {
		body["packageType"] = plan.PackageType.ValueString()
	} else {
		body["packageId"] = plan.PackageID.ValueInt64()
	}
	for k, v := range map[string]types.String{"email": plan.Email, "emailLanguage": plan.EmailLanguage, "companyName": plan.CompanyName,
		"notificationEmail": plan.NotificationEmail, "notificationPhoneNumber": plan.NotificationPhoneNumber} {
		if !v.IsNull() {
			body[k] = v.ValueString()
		}
	}
	if !plan.MonthlyReportsEnabled.IsUnknown() {
		body["monthlyReportsEnabled"] = plan.MonthlyReportsEnabled.ValueBool()
	}
	if !plan.CustomFields.IsUnknown() {
		cf, _ := parseJSONObject(plan.CustomFields.ValueString())
		body["customFields"] = cf
	}
	plan.overrideBody(body, nil)

	created, err := r.client.CreateCustomer(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating customer failed", err.Error())
		return
	}
	id := strconv.FormatInt(created.ID, 10)
	// Save the id first: whatever fails below, the customer exists and must stay in the state.
	plan.ID = types.StringValue(id)
	if !plan.overridesApplied(created) {
		resp.Diagnostics.AddError("Permission overrides not applied",
			fmt.Sprintf("Customer %s was created, but the API ignored the permission overrides: they need an organization token with write access.", id))
	}
	if !plan.NotificationTargets.IsNull() && !resp.Diagnostics.HasError() {
		t, d := targetsBody(ctx, plan.NotificationTargets)
		resp.Diagnostics.Append(d...)
		if err := r.client.UpdateCustomer(ctx, id, map[string]any{"notificationTargets": t}); err != nil {
			resp.Diagnostics.AddError("Setting notification targets failed", err.Error())
		}
	}
	r.readInto(ctx, id, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerResource) readInto(ctx context.Context, id string, m *customerResourceModel, diags *diag.Diagnostics) {
	c, err := r.client.GetCustomerDetail(ctx, id)
	if err != nil {
		diags.AddError("Reading customer failed", err.Error())
		return
	}
	diags.Append(m.fromAPI(ctx, c)...)
}

func (r *customerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetCustomerDetail(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading customer failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, c)...)
	if state.DeleteUserAction.IsNull() { // after import
		state.DeleteUserAction = types.StringValue("deactivate")
	}
	if state.CanEditManaged.IsNull() { // after an import without override access
		state.CanEditManaged = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Email.Equal(state.Email) {
		patch["email"] = plan.Email.ValueString() // "" clears it
	}
	if !plan.Status.Equal(state.Status) {
		patch["status"] = plan.Status.ValueString()
	}
	if plan.packageTypeChanged(&state) {
		patch["packageType"] = plan.PackageType.ValueString()
	} else if !plan.PackageID.IsUnknown() && !plan.PackageID.Equal(state.PackageID) {
		patch["packageId"] = plan.PackageID.ValueInt64Pointer()
	}
	if !plan.MonthlyReportsEnabled.IsUnknown() && !plan.MonthlyReportsEnabled.Equal(state.MonthlyReportsEnabled) {
		patch["monthlyReportsEnabled"] = plan.MonthlyReportsEnabled.ValueBool()
	}
	for k, p := range map[string][2]types.String{
		"emailLanguage":           {plan.EmailLanguage, state.EmailLanguage},
		"companyName":             {plan.CompanyName, state.CompanyName},
		"notificationEmail":       {plan.NotificationEmail, state.NotificationEmail},
		"notificationPhoneNumber": {plan.NotificationPhoneNumber, state.NotificationPhoneNumber},
	} {
		if !p[0].Equal(p[1]) {
			patch[k] = strOrNil(p[0])
		}
	}
	if !plan.CustomFields.IsUnknown() && !plan.CustomFields.Equal(state.CustomFields) {
		cf, _ := parseJSONObject(plan.CustomFields.ValueString())
		patch["customFields"] = cf
	}
	if !plan.NotificationTargets.Equal(state.NotificationTargets) {
		t, d := targetsBody(ctx, plan.NotificationTargets)
		resp.Diagnostics.Append(d...)
		patch["notificationTargets"] = t
	}
	plan.overrideBody(patch, &state)
	if len(patch) > 0 {
		if err := r.client.UpdateCustomer(ctx, state.ID.ValueString(), patch); err != nil {
			resp.Diagnostics.AddError("Updating customer failed", err.Error())
			return
		}
	}
	r.readInto(ctx, state.ID.ValueString(), &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteCustomer(ctx, state.ID.ValueString(), state.DeleteUserAction.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting customer failed", err.Error())
	}
}

func (r *customerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
