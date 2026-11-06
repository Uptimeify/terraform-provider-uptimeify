package provider

import (
	"context"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &imServiceResource{}
	_ resource.ResourceWithImportState = &imServiceResource{}

	imServiceMonitorType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"monitor_type": types.StringType, "monitor_id": types.Int64Type,
	}}
	// The API trims name and description and stores an empty description as null: both would
	// answer another value than the plan. (?s): a description may span lines.
	imServiceTrimmed = stringvalidator.RegexMatches(regexp.MustCompile(`(?s)^\S(.*\S)?$`), "must not be empty or start or end with whitespace")
)

type imServiceResource struct{ client *client.Client }

type imServiceModel struct {
	ID             types.String `tfsdk:"id"`
	PublicID       types.String `tfsdk:"public_id"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Criticality    types.String `tfsdk:"criticality"`
	OwnerTeamID    types.Int64  `tfsdk:"owner_team_id"`
	Monitors       types.Set    `tfsdk:"monitors"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (m *imServiceModel) fromAPI(s *client.IMService) {
	m.ID = types.StringValue(strconv.FormatInt(s.ID, 10))
	m.PublicID = types.StringValue(s.PublicID)
	m.OrganizationID = types.Int64Value(s.OrganizationID)
	m.Name = types.StringValue(s.Name)
	m.Description = types.StringPointerValue(s.Description)
	m.Criticality = types.StringValue(s.Criticality)
	m.OwnerTeamID = types.Int64PointerValue(s.OwnerTeamID)
	elems := make([]attr.Value, 0, len(s.Monitors))
	for _, mon := range s.Monitors {
		elems = append(elems, types.ObjectValueMust(imServiceMonitorType.AttrTypes, map[string]attr.Value{
			"monitor_type": types.StringValue(mon.MonitorType), "monitor_id": types.Int64Value(mon.MonitorID),
		}))
	}
	m.Monitors = types.SetValueMust(imServiceMonitorType, elems)
	m.CreatedAt = types.StringValue(s.CreatedAt)
	m.UpdatedAt = types.StringValue(s.UpdatedAt)
}

// imServiceBody holds the fields where plan differs from cur; with cur = nil it is the full create
// body. A null description or owner_team_id goes out as JSON null, which clears it.
func imServiceBody(ctx context.Context, plan, cur *imServiceModel) (map[string]any, diag.Diagnostics) {
	body := map[string]any{}
	if cur == nil || !plan.Name.Equal(cur.Name) {
		body["name"] = plan.Name.ValueString()
	}
	if cur == nil || !plan.Description.Equal(cur.Description) {
		body["description"] = plan.Description.ValueStringPointer()
	}
	if cur == nil || !plan.Criticality.Equal(cur.Criticality) {
		body["criticality"] = plan.Criticality.ValueString()
	}
	if cur == nil || !plan.OwnerTeamID.Equal(cur.OwnerTeamID) {
		body["ownerTeamId"] = plan.OwnerTeamID.ValueInt64Pointer()
	}
	if cur != nil && plan.Monitors.Equal(cur.Monitors) {
		return body, nil
	}
	var mons []struct {
		Type types.String `tfsdk:"monitor_type"`
		ID   types.Int64  `tfsdk:"monitor_id"`
	}
	diags := plan.Monitors.ElementsAs(ctx, &mons, false)
	refs := make([]client.IMServiceMonitor, 0, len(mons))
	for _, mon := range mons {
		refs = append(refs, client.IMServiceMonitor{MonitorType: mon.Type.ValueString(), MonitorID: mon.ID.ValueInt64()})
	}
	body["monitors"] = refs
	return body, diags
}

func NewIMServiceResource() resource.Resource { return &imServiceResource{} }

func (r *imServiceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_service"
}

func (r *imServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A service of the Incident Management service catalog (\"Webshop\", \"Customer portal\") with its criticality, the responsible team and its monitors. " +
			"An incident of a monitor that belongs to exactly one service is linked to that service. Needs an organization token of an organization admin. " +
			"Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Numeric service id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"organization_id": schema.Int64Attribute{Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "1 to 200 characters, unique per organization regardless of case (409 `imServiceNameTaken`).",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 200), imServiceTrimmed}},
			"description": schema.StringAttribute{Optional: true, Description: "Up to 2000 characters.",
				Validators: []validator.String{stringvalidator.LengthAtMost(2000), imServiceTrimmed}},
			"criticality": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("medium"),
				Description: "`critical`, `high`, `medium` (default) or `low`.",
				Validators:  []validator.String{stringvalidator.OneOf("critical", "high", "medium", "low")}},
			"owner_team_id": schema.Int64Attribute{Optional: true, Description: "Responsible team of the organization (404 `imTeamNotFound`). Deleting the team clears it.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"monitors": schema.SetNestedAttribute{
				Optional: true, Computed: true, Default: setdefault.StaticValue(types.SetValueMust(imServiceMonitorType, nil)),
				Description: "The monitors of this service, at most 500. Terraform owns the whole set; unset = none. Each must belong to the organization (404 `imServiceMonitorNotFound`). " +
					"A monitor deleted later drops out of the service and shows up as a diff.",
				Validators: []validator.Set{setvalidator.SizeAtMost(500)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"monitor_type": schema.StringAttribute{Required: true, Description: "`website` (also heartbeat and browser monitors), `dns`, `icmp`, `smtp`, `ssh`, `tcp`, `ftp` or `imap_pop`.",
						Validators: []validator.String{stringvalidator.OneOf("website", "dns", "icmp", "smtp", "ssh", "tcp", "ftp", "imap_pop")}},
					"monitor_id": schema.Int64Attribute{Required: true, Description: "Numeric id of the monitor within its type, e.g. `tonumber(uptimeify_website.shop.id)`.",
						Validators: []validator.Int64{int64validator.AtLeast(1)}},
				}},
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *imServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *imServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, diags := imServiceBody(ctx, &plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.CreateIMService(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating IM service failed", err.Error())
		return
	}
	plan.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetIMService(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading IM service failed", err.Error())
		return
	}
	state.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, diags := imServiceBody(ctx, &plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.UpdateIMService(ctx, state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating IM service failed", err.Error())
		return
	}
	plan.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMService(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting IM service failed", err.Error())
	}
}

func (r *imServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
