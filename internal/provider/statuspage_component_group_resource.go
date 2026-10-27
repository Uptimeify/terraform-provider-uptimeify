package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.ResourceWithConfigure   = &statusPageComponentGroupResource{}
	_ resource.ResourceWithImportState = &statusPageComponentGroupResource{}
)

type statusPageComponentGroupResource struct{ client *client.Client }

type statusPageComponentGroupModel struct {
	ID           types.String `tfsdk:"id"`
	StatusPageID types.Int64  `tfsdk:"status_page_id"`
	GroupID      types.Int64  `tfsdk:"group_id"`
	PublicID     types.String `tfsdk:"public_id"`
	Name         types.String `tfsdk:"name"`
	Sort         types.Int64  `tfsdk:"sort"`
	Collapsed    types.Bool   `tfsdk:"collapsed"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (m *statusPageComponentGroupModel) fromAPI(g *client.StatusPageComponentGroup) {
	m.ID = types.StringValue(idPair(g.StatusPageID, g.ID))
	m.StatusPageID = types.Int64Value(g.StatusPageID)
	m.GroupID = types.Int64Value(g.ID)
	m.PublicID = types.StringValue(g.PublicID)
	m.Name = types.StringValue(g.Name)
	m.Sort = types.Int64Value(g.Sort)
	m.Collapsed = types.BoolValue(g.Collapsed)
	m.CreatedAt = types.StringValue(g.CreatedAt)
}

func NewStatusPageComponentGroupResource() resource.Resource {
	return &statusPageComponentGroupResource{}
}

func (r *statusPageComponentGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component_group"
}

func (r *statusPageComponentGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A group of status page components (set `group_id` on `uptimeify_status_page_component`). Destroying a group " +
			"keeps its components, ungrouped. Import with `<status_page_id>/<group_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "`<status_page_id>/<group_id>`."},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id. Changing it replaces the group.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"group_id":  schema.Int64Attribute{Computed: true, PlanModifiers: keepI, Description: "Numeric group id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"name": schema.StringAttribute{Required: true, Description: "1 to 120 characters.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 120), statusPageTrimmed}},
			"sort": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Position, ascending, 0 to 100000. Unset = after the last group at creation.",
				Validators:  []validator.Int64{int64validator.Between(0, 100000)}},
			"collapsed": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Shown collapsed on the public page. Default `false`."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
		},
	}
}

func (r *statusPageComponentGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageComponentGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageComponentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": plan.Name.ValueString(), "collapsed": plan.Collapsed.ValueBool()}
	if !plan.Sort.IsUnknown() && !plan.Sort.IsNull() {
		body["sort"] = plan.Sort.ValueInt64()
	}
	g, err := r.client.CreateStatusPageComponentGroup(ctx, plan.StatusPageID.ValueInt64(), body)
	if err != nil {
		resp.Diagnostics.AddError("Creating status page component group failed", err.Error())
		return
	}
	plan.fromAPI(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageComponentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetStatusPageComponentGroup(ctx, state.StatusPageID.ValueInt64(), state.GroupID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page component group failed", err.Error())
		return
	}
	state.fromAPI(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageComponentGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusPageComponentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Sort.IsUnknown() && !plan.Sort.Equal(state.Sort) {
		patch["sort"] = plan.Sort.ValueInt64()
	}
	if !plan.Collapsed.Equal(state.Collapsed) {
		patch["collapsed"] = plan.Collapsed.ValueBool()
	}
	pageID, id := state.StatusPageID.ValueInt64(), state.GroupID.ValueInt64()
	var g *client.StatusPageComponentGroup
	var err error
	if len(patch) == 0 { // the API refuses an empty PATCH
		g, err = r.client.GetStatusPageComponentGroup(ctx, pageID, id)
	} else {
		g, err = r.client.UpdateStatusPageComponentGroup(ctx, pageID, id, patch)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating status page component group failed", err.Error())
		return
	}
	plan.fromAPI(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageComponentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteStatusPageComponentGroup(ctx, state.StatusPageID.ValueInt64(), state.GroupID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page component group failed", err.Error())
	}
}

func (r *statusPageComponentGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, id, err := parseTwoIDs(req.ID, "<status_page_id>/<group_id>")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), id)...)
}
