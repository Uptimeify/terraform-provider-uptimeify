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
	_ resource.ResourceWithConfigure   = &statusPageComponentResource{}
	_ resource.ResourceWithImportState = &statusPageComponentResource{}
)

type statusPageComponentResource struct{ client *client.Client }

type statusPageComponentModel struct {
	ID             types.String `tfsdk:"id"`
	StatusPageID   types.Int64  `tfsdk:"status_page_id"`
	ComponentID    types.Int64  `tfsdk:"component_id"`
	PublicID       types.String `tfsdk:"public_id"`
	SourceKind     types.String `tfsdk:"source_kind"`
	MonitorType    types.String `tfsdk:"monitor_type"`
	MonitorID      types.Int64  `tfsdk:"monitor_id"`
	DependencySlug types.String `tfsdk:"dependency_slug"`
	ManualStatus   types.String `tfsdk:"manual_status"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	GroupID        types.Int64  `tfsdk:"group_id"`
	Sort           types.Int64  `tfsdk:"sort"`
	ShowUptime     types.Bool   `tfsdk:"show_uptime"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (m *statusPageComponentModel) fromAPI(c *client.StatusPageComponent) {
	m.ID = types.StringValue(idPair(c.StatusPageID, c.ID))
	m.StatusPageID = types.Int64Value(c.StatusPageID)
	m.ComponentID = types.Int64Value(c.ID)
	m.PublicID = types.StringValue(c.PublicID)
	m.SourceKind = types.StringValue(c.SourceKind)
	m.MonitorType = types.StringPointerValue(c.MonitorType)
	m.MonitorID = types.Int64PointerValue(c.MonitorID)
	m.DependencySlug = types.StringPointerValue(c.DependencySlug)
	m.ManualStatus = types.StringPointerValue(c.ManualStatus)
	m.Name = types.StringValue(c.Name)
	m.Description = types.StringPointerValue(c.Description)
	m.GroupID = types.Int64PointerValue(c.GroupID)
	m.Sort = types.Int64Value(c.Sort)
	m.ShowUptime = types.BoolValue(c.ShowUptime)
	m.CreatedAt = types.StringValue(c.CreatedAt)
}

func NewStatusPageComponentResource() resource.Resource { return &statusPageComponentResource{} }

func (r *statusPageComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component"
}

func (r *statusPageComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	replaceS := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "One component (row) of a status page: a monitor, a manually set status, or a third-party provider " +
			"(`dependency`) whose published status it follows. At most 100 per page. Import with `<status_page_id>/<component_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "`<status_page_id>/<component_id>`."},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id. Changing it replaces the component.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"component_id": schema.Int64Attribute{Computed: true, PlanModifiers: keepI, Description: "Numeric component id."},
			"public_id":    schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"source_kind": schema.StringAttribute{Required: true, PlanModifiers: replaceS,
				Description: "`monitor`, `manual` or `dependency`. Changing it replaces the component.",
				Validators:  []validator.String{stringvalidator.OneOf(client.ComponentSourceKinds...)}},
			"monitor_type": schema.StringAttribute{Optional: true, PlanModifiers: replaceS,
				Description: "With `source_kind = \"monitor\"`: `http` (website), `dns`, `icmp`, `smtp`, `ssh`, `tcp`, `ftp` or `imap_pop`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.MonitorKinds...)}},
			"monitor_id": schema.Int64Attribute{Optional: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Description: "With `source_kind = \"monitor\"`: numeric monitor id, a monitor of the page's customer. One component per monitor and page.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)}},
			"dependency_slug": schema.StringAttribute{Optional: true, PlanModifiers: replaceS,
				Description: "With `source_kind = \"dependency\"`: provider slug from `GET /api/public/dependencies`, e.g. `github`. One component per provider and page.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 120)}},
			"manual_status": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "With `source_kind = \"manual\"` only: `operational` (default), `degraded`, `partial_outage`, `major_outage` or `maintenance`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.ComponentManualStatuses...)}},
			"name": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "1 to 120 characters. Required for `manual`; defaults to the monitor or provider name.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 120), statusPageTrimmed}},
			"description": schema.StringAttribute{Optional: true, Description: "Up to 1000 characters.",
				Validators: []validator.String{stringvalidator.LengthAtMost(1000), statusPageTrimmed}},
			"group_id": schema.Int64Attribute{Optional: true, Description: "Numeric id of an `uptimeify_status_page_component_group` of the same page. Unset = no group.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"sort": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Position, ascending, 0 to 100000. Unset = after the last component at creation.",
				Validators:  []validator.Int64{int64validator.Between(0, 100000)}},
			"show_uptime": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Default `true`."},
			"created_at":  schema.StringAttribute{Computed: true, PlanModifiers: keepS},
		},
	}
}

func (r *statusPageComponentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"sourceKind": plan.SourceKind.ValueString(), "showUptime": plan.ShowUptime.ValueBool()}
	setIfKnown := func(key string, v interface {
		IsNull() bool
		IsUnknown() bool
	}, val any) {
		if !v.IsNull() && !v.IsUnknown() {
			body[key] = val
		}
	}
	setIfKnown("monitorType", plan.MonitorType, plan.MonitorType.ValueString())
	setIfKnown("monitorId", plan.MonitorID, plan.MonitorID.ValueInt64())
	setIfKnown("dependencySlug", plan.DependencySlug, plan.DependencySlug.ValueString())
	setIfKnown("manualStatus", plan.ManualStatus, plan.ManualStatus.ValueString())
	setIfKnown("name", plan.Name, plan.Name.ValueString())
	setIfKnown("description", plan.Description, plan.Description.ValueString())
	setIfKnown("groupId", plan.GroupID, plan.GroupID.ValueInt64())
	setIfKnown("sort", plan.Sort, plan.Sort.ValueInt64())
	c, err := r.client.CreateStatusPageComponent(ctx, plan.StatusPageID.ValueInt64(), body)
	if err != nil {
		resp.Diagnostics.AddError("Creating status page component failed", err.Error())
		return
	}
	plan.fromAPI(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetStatusPageComponent(ctx, state.StatusPageID.ValueInt64(), state.ComponentID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page component failed", err.Error())
		return
	}
	state.fromAPI(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.IsUnknown() && !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Description.Equal(state.Description) {
		patch["description"] = plan.Description.ValueStringPointer() // nil clears it
	}
	if !plan.GroupID.Equal(state.GroupID) {
		patch["groupId"] = plan.GroupID.ValueInt64Pointer() // nil = out of its group
	}
	if !plan.Sort.IsUnknown() && !plan.Sort.Equal(state.Sort) {
		patch["sort"] = plan.Sort.ValueInt64()
	}
	if !plan.ShowUptime.Equal(state.ShowUptime) {
		patch["showUptime"] = plan.ShowUptime.ValueBool()
	}
	if !plan.ManualStatus.IsUnknown() && !plan.ManualStatus.IsNull() && !plan.ManualStatus.Equal(state.ManualStatus) {
		patch["manualStatus"] = plan.ManualStatus.ValueString()
	}
	pageID, id := state.StatusPageID.ValueInt64(), state.ComponentID.ValueInt64()
	var c *client.StatusPageComponent
	var err error
	if len(patch) == 0 { // the API refuses an empty PATCH
		c, err = r.client.GetStatusPageComponent(ctx, pageID, id)
	} else {
		c, err = r.client.UpdateStatusPageComponent(ctx, pageID, id, patch)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating status page component failed", err.Error())
		return
	}
	plan.fromAPI(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteStatusPageComponent(ctx, state.StatusPageID.ValueInt64(), state.ComponentID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page component failed", err.Error())
	}
}

func (r *statusPageComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, id, err := parseTwoIDs(req.ID, "<status_page_id>/<component_id>")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("component_id"), id)...)
}
