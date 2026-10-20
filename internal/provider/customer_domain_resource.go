package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &customerDomainResource{}
	_ resource.ResourceWithImportState = &customerDomainResource{}
)

type customerDomainResource struct{ client *client.Client }

type customerDomainModel struct {
	ID                types.String `tfsdk:"id"`
	PublicID          types.String `tfsdk:"public_id"`
	CustomerID        types.Int64  `tfsdk:"customer_id"`
	DomainName        types.String `tfsdk:"domain_name"`
	Label             types.String `tfsdk:"label"`
	Status            types.String `tfsdk:"status"`
	ManagementType    types.String `tfsdk:"management_type"`
	ExpiryWarningDays types.Int64  `tfsdk:"expiry_warning_days"`
	ExpiryErrorDays   types.Int64  `tfsdk:"expiry_error_days"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func (m *customerDomainModel) fromAPI(d *client.CustomerDomain) {
	m.ID = types.StringValue(strconv.FormatInt(d.ID, 10))
	m.PublicID = types.StringValue(d.PublicID)
	m.CustomerID = types.Int64Value(d.CustomerID)
	m.DomainName = types.StringValue(d.DomainName)
	m.Label = nonEmpty(d.Label)
	m.Status = types.StringValue(d.Status)
	m.ManagementType = types.StringValue(d.ManagementType)
	m.ExpiryWarningDays = types.Int64Value(d.ExpiryWarningDays)
	m.ExpiryErrorDays = types.Int64Value(d.ExpiryErrorDays)
	m.CreatedAt = types.StringValue(d.CreatedAt)
	m.UpdatedAt = types.StringValue(d.UpdatedAt)
}

func NewCustomerDomainResource() resource.Resource { return &customerDomainResource{} }

func (r *customerDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_domain"
}

func (r *customerDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A domain of a customer whose registration expiry is watched over RDAP. The top-level domain must " +
			"support RDAP. Creating one that already exists with the same values fails; import it instead. " +
			"Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"customer_id": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"domain_name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description: "Registered domain without protocol, lower case, e.g. `example.com`.",
				Validators: []validator.String{stringvalidator.LengthAtMost(253),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`), "must be a lower-case domain name")}},
			"label": schema.StringAttribute{Optional: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(255)}},
			"status": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("active"),
				Description: "`active` (checked, counts against the monitor quota) or `disabled`.", Validators: []validator.String{monitorEntryStatus}},
			"management_type": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: managementTypeDesc, Validators: []validator.String{monitorManagementTypes}},
			"expiry_warning_days": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(30),
				Description: "Warn this many days before expiry, 1 to 365.", Validators: []validator.Int64{int64validator.Between(1, 365)}},
			"expiry_error_days": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(7),
				Description: "Alert this many days before expiry, 1 to 90.", Validators: []validator.Int64{int64validator.Between(1, 90)}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *customerDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *customerDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customerDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The create route takes the customer's public id only.
	cu, err := r.client.GetCustomer(ctx, strconv.FormatInt(plan.CustomerID.ValueInt64(), 10))
	if err != nil {
		resp.Diagnostics.AddError("Reading customer failed", err.Error())
		return
	}
	body := map[string]any{
		"domainName":        plan.DomainName.ValueString(),
		"status":            plan.Status.ValueString(),
		"expiryWarningDays": plan.ExpiryWarningDays.ValueInt64(),
		"expiryErrorDays":   plan.ExpiryErrorDays.ValueInt64(),
	}
	if !plan.Label.IsNull() {
		body["label"] = plan.Label.ValueString()
	}
	if !plan.ManagementType.IsUnknown() {
		body["managementType"] = plan.ManagementType.ValueString()
	}
	d, err := r.client.CreateCustomerDomain(ctx, cu.PublicID, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating customer domain failed", err.Error())
		return
	}
	if d.Deduplicated {
		resp.Diagnostics.AddError("Customer domain already exists",
			fmt.Sprintf("The customer already has this domain with the same values (id %d). Import it: terraform import <address> %d", d.ID, d.ID))
		return
	}
	plan.fromAPI(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customerDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetCustomerDomain(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading customer domain failed", err.Error())
		return
	}
	state.fromAPI(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customerDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customerDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Label.Equal(state.Label) {
		patch["label"] = strOrNil(plan.Label)
	}
	if !plan.Status.Equal(state.Status) {
		patch["status"] = plan.Status.ValueString()
	}
	if !plan.ManagementType.IsUnknown() && !plan.ManagementType.Equal(state.ManagementType) {
		patch["managementType"] = plan.ManagementType.ValueString()
	}
	if !plan.ExpiryWarningDays.Equal(state.ExpiryWarningDays) {
		patch["expiryWarningDays"] = plan.ExpiryWarningDays.ValueInt64()
	}
	if !plan.ExpiryErrorDays.Equal(state.ExpiryErrorDays) {
		patch["expiryErrorDays"] = plan.ExpiryErrorDays.ValueInt64()
	}
	d, err := r.client.UpdateCustomerDomain(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating customer domain failed", err.Error())
		return
	}
	plan.fromAPI(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customerDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteCustomerDomain(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting customer domain failed", err.Error())
	}
}

func (r *customerDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
