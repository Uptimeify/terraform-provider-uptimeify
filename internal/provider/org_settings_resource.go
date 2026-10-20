package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &orgSettingsResource{}
	_ resource.ResourceWithImportState = &orgSettingsResource{}
)

var (
	orgChannelsAttrTypes = map[string]attr.Type{"email": types.BoolType, "sms": types.BoolType, "webhook": types.BoolType, "integrations": types.BoolType}
	orgTargetsAttrTypes  = map[string]attr.Type{"email": types.StringType, "sms": types.StringType, "webhook": types.StringType, "integrations": types.StringType}
	orgTargetAudiences   = []string{"customer", "organization", "both"}
)

type orgSettingsResource struct{ client *client.Client }

type orgSettingsModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	CompanyName                     types.String `tfsdk:"company_name"`
	Street                          types.String `tfsdk:"street"`
	PostalCode                      types.String `tfsdk:"postal_code"`
	City                            types.String `tfsdk:"city"`
	Country                         types.String `tfsdk:"country"`
	VatID                           types.String `tfsdk:"vat_id"`
	BillingEmail                    types.String `tfsdk:"billing_email"`
	RequireMfaAdmins                types.Bool   `tfsdk:"require_mfa_admins"`
	RequireMfaEditors               types.Bool   `tfsdk:"require_mfa_editors"`
	RequireMfaReadonly              types.Bool   `tfsdk:"require_mfa_readonly"`
	RequireMfaCustomers             types.Bool   `tfsdk:"require_mfa_customers"`
	SmsOverageSafetyCapCentsMonthly types.Int64  `tfsdk:"sms_overage_safety_cap_cents_monthly"`
	DefaultNotificationChannels     types.Object `tfsdk:"default_notification_channels"`
	DefaultNotificationTargets      types.Object `tfsdk:"default_notification_targets"`
}

func (m *orgSettingsModel) fromAPI(ctx context.Context, o *client.Organization) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(strconv.FormatInt(o.ID, 10))
	m.Name = types.StringValue(o.Name)
	m.CompanyName = types.StringPointerValue(o.CompanyName)
	m.Street = types.StringPointerValue(o.Street)
	m.PostalCode = types.StringPointerValue(o.PostalCode)
	m.City = types.StringPointerValue(o.City)
	m.Country = types.StringPointerValue(o.Country)
	m.VatID = types.StringPointerValue(o.VatID)
	m.BillingEmail = types.StringPointerValue(o.BillingEmail)
	m.RequireMfaAdmins = types.BoolPointerValue(o.RequireMfaAdmins)
	m.RequireMfaEditors = types.BoolPointerValue(o.RequireMfaEditors)
	m.RequireMfaReadonly = types.BoolPointerValue(o.RequireMfaReadonly)
	m.RequireMfaCustomers = types.BoolPointerValue(o.RequireMfaCustomers)
	m.SmsOverageSafetyCapCentsMonthly = types.Int64PointerValue(o.SmsOverageSafetyCapCentsMonthly)
	var d diag.Diagnostics
	m.DefaultNotificationChannels, d = types.ObjectValueFrom(ctx, orgChannelsAttrTypes, o.DefaultNotificationChannels)
	diags.Append(d...)
	m.DefaultNotificationTargets, d = types.ObjectValueFrom(ctx, orgTargetsAttrTypes, o.DefaultNotificationTargets)
	diags.Append(d...)
	return diags
}

func NewOrgSettingsResource() resource.Resource { return &orgSettingsResource{} }

func (r *orgSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_settings"
}

func (r *orgSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	str := func(desc string, v ...validator.String) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Description: desc + " Unset = keep the current value.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Validators: v}
	}
	mfa := func(audience string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true,
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			Description: "Require two-step verification for " + audience + ". Changing it needs a signed-in admin who has " +
				"two-step verification: API tokens are refused (`nonUserSessionForbidden`). An unchanged value is never sent. Unset = keep."}
	}
	obj := []planmodifier.Object{objectplanmodifier.UseStateForUnknown()}
	target := func() schema.StringAttribute {
		return schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf(orgTargetAudiences...)}}
	}
	resp.Schema = schema.Schema{
		Description: "Settings of the token's own organization (one per organization): name, billing address, MFA policy, " +
			"SMS overage cap and notification defaults for new customers. Only the attributes you set are managed and sent, " +
			"and only when they differ from the stored value. Creating it adopts the organization; destroying it only removes " +
			"it from the state, nothing is reset. Plan, quota and payment are not managed here. Import with any id, e.g. `org`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The organization id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":          str("Organization name.", stringvalidator.LengthBetween(1, 255)),
			"company_name":  str("Company name on invoices."),
			"street":        str("Billing address."),
			"postal_code":   str("Billing address."),
			"city":          str("Billing address."),
			"country":       str("Billing country (free text; the server derives the ISO code). Changing it resets the VAT check."),
			"vat_id":        str("VAT ID. Changing it resets the VIES verdict until it is validated again."),
			"billing_email": str("Address for invoices (admins only)."),
			"require_mfa_admins":    mfa("admins"),
			"require_mfa_editors":   mfa("team editors"),
			"require_mfa_readonly":  mfa("team readonly members"),
			"require_mfa_customers": mfa("customer users (logins limited to customers)"),
			"sms_overage_safety_cap_cents_monthly": schema.Int64Attribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:    []validator.Int64{int64validator.AtLeast(0)},
				Description: "Monthly SMS overage cap in cents. Only global admins may change it (`forbidden` for organization " +
					"tokens); an unchanged value is never sent. Clearing it (null) is not possible here. Unset = keep."},
			"default_notification_channels": schema.SingleNestedAttribute{Optional: true, Computed: true, PlanModifiers: obj,
				Description: "Channels switched on for new customers. Unset = keep.",
				Attributes: map[string]schema.Attribute{
					"email":        schema.BoolAttribute{Required: true},
					"sms":          schema.BoolAttribute{Required: true},
					"webhook":      schema.BoolAttribute{Required: true},
					"integrations": schema.BoolAttribute{Required: true},
				}},
			"default_notification_targets": schema.SingleNestedAttribute{Optional: true, Computed: true, PlanModifiers: obj,
				Description: "Who receives each channel for new customers: `customer`, `organization` or `both` (`webhook`: `organization` only). Unset = keep.",
				Attributes: map[string]schema.Attribute{
					"email":        target(),
					"sms":          target(),
					"webhook":      schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf("organization")}},
					"integrations": target(),
				}},
		},
	}
}

func (r *orgSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func setStr(patch map[string]any, key string, plan types.String, cur *string) {
	if plan.IsNull() || plan.IsUnknown() || (cur != nil && *cur == plan.ValueString()) {
		return
	}
	patch[key] = plan.ValueString()
}

func setBool(patch map[string]any, key string, plan types.Bool, cur *bool) {
	if plan.IsNull() || plan.IsUnknown() || (cur != nil && *cur == plan.ValueBool()) {
		return
	}
	patch[key] = plan.ValueBool()
}

// setObj sends a configured nested object whole when it differs from the stored one.
func setObj(ctx context.Context, patch map[string]any, key string, plan types.Object, cur basetypes.ObjectValue, out any) diag.Diagnostics {
	if plan.IsNull() || plan.IsUnknown() || plan.Equal(cur) {
		return nil
	}
	d := plan.As(ctx, out, basetypes.ObjectAsOptions{})
	patch[key] = out
	return d
}

// apply PATCHes the configured values that differ from the stored organization. The server
// drops guarded fields whose value does not change as well, but an empty body is avoided.
func (r *orgSettingsResource) apply(ctx context.Context, plan *orgSettingsModel) diag.Diagnostics {
	var diags diag.Diagnostics
	cur, err := r.client.GetOrganization(ctx)
	if err != nil {
		diags.AddError("Reading organization failed", err.Error())
		return diags
	}
	var stored orgSettingsModel
	diags.Append(stored.fromAPI(ctx, cur)...)

	patch := map[string]any{}
	setStr(patch, "name", plan.Name, &cur.Name)
	setStr(patch, "companyName", plan.CompanyName, cur.CompanyName)
	setStr(patch, "street", plan.Street, cur.Street)
	setStr(patch, "postalCode", plan.PostalCode, cur.PostalCode)
	setStr(patch, "city", plan.City, cur.City)
	setStr(patch, "country", plan.Country, cur.Country)
	setStr(patch, "vatId", plan.VatID, cur.VatID)
	setStr(patch, "billingEmail", plan.BillingEmail, cur.BillingEmail)
	setBool(patch, "requireMfaAdmins", plan.RequireMfaAdmins, cur.RequireMfaAdmins)
	setBool(patch, "requireMfaEditors", plan.RequireMfaEditors, cur.RequireMfaEditors)
	setBool(patch, "requireMfaReadonly", plan.RequireMfaReadonly, cur.RequireMfaReadonly)
	setBool(patch, "requireMfaCustomers", plan.RequireMfaCustomers, cur.RequireMfaCustomers)
	if v := plan.SmsOverageSafetyCapCentsMonthly; !v.IsNull() && !v.IsUnknown() && !v.Equal(stored.SmsOverageSafetyCapCentsMonthly) {
		patch["smsOverageSafetyCapCentsMonthly"] = v.ValueInt64()
	}
	diags.Append(setObj(ctx, patch, "defaultNotificationChannels", plan.DefaultNotificationChannels, stored.DefaultNotificationChannels, &client.NotificationChannels{})...)
	diags.Append(setObj(ctx, patch, "defaultNotificationTargets", plan.DefaultNotificationTargets, stored.DefaultNotificationTargets, &client.NotificationTargets{})...)
	if diags.HasError() {
		return diags
	}
	if len(patch) > 0 {
		if cur, err = r.client.UpdateOrganization(ctx, patch); err != nil {
			diags.AddError("Updating organization settings failed", orgSettingsHint(err))
			return diags
		}
	}
	diags.Append(plan.fromAPI(ctx, cur)...)
	return diags
}

func orgSettingsHint(err error) string {
	if apiErr, ok := err.(*client.APIError); ok {
		switch apiErr.Code {
		case "nonUserSessionForbidden":
			return err.Error() + "\nThe MFA switches can only be changed by a signed-in admin, not with an API token."
		case "mfaRequiredForActor":
			return err.Error() + "\nEnable two-step verification on your own account first."
		}
	}
	return err.Error()
}

func (r *orgSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.apply(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *orgSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading organization failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, o)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *orgSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan orgSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.apply(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete leaves the organization as it is.
func (r *orgSettingsResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

func (r *orgSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
