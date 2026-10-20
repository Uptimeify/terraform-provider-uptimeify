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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &customerIPResource{}
	_ resource.ResourceWithImportState = &customerIPResource{}
)

type customerIPResource struct{ client *client.Client }

type customerIPModel struct {
	ID             types.String `tfsdk:"id"`
	PublicID       types.String `tfsdk:"public_id"`
	CustomerID     types.Int64  `tfsdk:"customer_id"`
	IPAddress      types.String `tfsdk:"ip_address"`
	IPFamily       types.String `tfsdk:"ip_family"`
	Label          types.String `tfsdk:"label"`
	Status         types.String `tfsdk:"status"`
	ManagementType types.String `tfsdk:"management_type"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (m *customerIPModel) fromAPI(ip *client.CustomerIP) {
	m.ID = types.StringValue(strconv.FormatInt(ip.ID, 10))
	m.PublicID = types.StringValue(ip.PublicID)
	m.CustomerID = types.Int64Value(ip.CustomerID)
	m.IPAddress = types.StringValue(ip.IPAddress)
	m.IPFamily = types.StringValue(ip.IPFamily)
	m.Label = nonEmpty(ip.Label)
	m.Status = types.StringValue(ip.Status)
	m.ManagementType = types.StringValue(ip.ManagementType)
	m.CreatedAt = types.StringValue(ip.CreatedAt)
	m.UpdatedAt = types.StringValue(ip.UpdatedAt)
}

// Shared by the IP and domain resources.
var (
	monitorEntryStatus     = stringvalidator.OneOf("active", "disabled")
	monitorManagementTypes = stringvalidator.OneOf("managed", "self_service")
)

const managementTypeDesc = "`managed` or `self_service`. Only organization writers choose it; customer-scoped tokens always " +
	"create `self_service` entries, everyone else gets `managed` when unset."

func NewCustomerIPResource() resource.Resource { return &customerIPResource{} }

func (r *customerIPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_ip"
}

func (r *customerIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "An IP address of a customer, checked against DNS blocklists (DNSBL). Creating one that already exists " +
			"with the same values fails; import it instead. Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"customer_id": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"ip_address": schema.StringAttribute{Required: true, PlanModifiers: replace,
				Description: "IPv4 or IPv6 address, lower case.",
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f.:]{2,128}$`), "must be an IP address in lower case")}},
			"ip_family": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
				Description:   "`v4` or `v6`; derived from the address when unset.", Validators: []validator.String{stringvalidator.OneOf("v4", "v6")}},
			"label": schema.StringAttribute{Optional: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(255)}},
			"status": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("active"),
				Description: "`active` (checked, counts against the IP quota) or `disabled`.", Validators: []validator.String{monitorEntryStatus}},
			"management_type": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep,
				Description: managementTypeDesc, Validators: []validator.String{monitorManagementTypes}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *customerIPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *customerIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customerIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"ipAddress": plan.IPAddress.ValueString(), "status": plan.Status.ValueString()}
	if !plan.IPFamily.IsUnknown() {
		body["ipFamily"] = plan.IPFamily.ValueString()
	}
	if !plan.Label.IsNull() {
		body["label"] = plan.Label.ValueString()
	}
	if !plan.ManagementType.IsUnknown() {
		body["managementType"] = plan.ManagementType.ValueString()
	}
	ip, err := r.client.CreateCustomerIP(ctx, strconv.FormatInt(plan.CustomerID.ValueInt64(), 10), body)
	if err != nil {
		resp.Diagnostics.AddError("Creating customer IP failed", err.Error())
		return
	}
	if ip.Deduplicated {
		resp.Diagnostics.AddError("Customer IP already exists",
			fmt.Sprintf("The customer already has this IP with the same values (id %d). Import it: terraform import <address> %d", ip.ID, ip.ID))
		return
	}
	plan.fromAPI(ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customerIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ip, err := r.client.GetCustomerIP(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading customer IP failed", err.Error())
		return
	}
	state.fromAPI(ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customerIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customerIPModel
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
	ip, err := r.client.UpdateCustomerIP(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating customer IP failed", err.Error())
		return
	}
	plan.fromAPI(ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customerIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customerIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteCustomerIP(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting customer IP failed", err.Error())
	}
}

func (r *customerIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
