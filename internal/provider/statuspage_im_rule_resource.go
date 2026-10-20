package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure     = &imStatuspageRuleResource{}
	_ resource.ResourceWithImportState   = &imStatuspageRuleResource{}
	_ datasource.DataSourceWithConfigure = &imStatuspageRuleDataSource{}
)

type imStatuspageRuleResource struct{ client *client.Client }

// imStatuspageRuleModel is shared by the uptimeify_im_statuspage_rule resource and data source.
type imStatuspageRuleModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	TeamID         types.Int64  `tfsdk:"team_id"`
	StatusPageID   types.Int64  `tfsdk:"status_page_id"`
	MinSeverity    types.String `tfsdk:"min_severity"`
	TargetState    types.String `tfsdk:"target_state"`
	AutoResolve    types.Bool   `tfsdk:"auto_resolve"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (m *imStatuspageRuleModel) fromAPI(r *client.ImStatuspageRule) {
	m.ID = types.StringValue(strconv.FormatInt(r.ID, 10))
	m.OrganizationID = types.Int64Value(r.OrganizationID)
	m.TeamID = types.Int64Value(r.TeamID)
	m.StatusPageID = types.Int64Value(r.StatusPageID)
	m.MinSeverity = types.StringValue(r.MinSeverity)
	m.TargetState = types.StringValue(r.TargetState)
	m.AutoResolve = types.BoolValue(r.AutoResolve)
	m.CreatedAt = types.StringValue(r.CreatedAt)
	m.UpdatedAt = types.StringValue(r.UpdatedAt)
}

func NewImStatuspageRuleResource() resource.Resource { return &imStatuspageRuleResource{} }

func (r *imStatuspageRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_statuspage_rule"
}

func (r *imStatuspageRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An Incident Management status page rule: incidents of `team_id` at `min_severity` or worse set the " +
			"status page to `target_state`. Needs team-admin rights on the team (or organization admin). Manual overrides " +
			"(`POST /api/im/statuspage-rules/override`) are an incident-time action and not managed by Terraform. " +
			"Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric rule id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Required: true, Description: "Incident Management team of your organization.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id of your organization.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"min_severity": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("sev2"),
				Description: "`sev1` (most severe) to `sev4`. Default `sev2`.", Validators: sevValidator},
			"target_state": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("degraded"),
				Description: "`warning` or `degraded` (default).",
				Validators:  []validator.String{stringvalidator.OneOf(client.StatusPageTargetStates...)}},
			"auto_resolve": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Reset the page when the incident resolves. Default `true`."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *imStatuspageRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *imStatuspageRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imStatuspageRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.CreateImStatuspageRule(ctx, map[string]any{
		"teamId": plan.TeamID.ValueInt64(), "statusPageId": plan.StatusPageID.ValueInt64(),
		"minSeverity": plan.MinSeverity.ValueString(), "targetState": plan.TargetState.ValueString(),
		"autoResolve": plan.AutoResolve.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating status page rule failed", err.Error())
		return
	}
	plan.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imStatuspageRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imStatuspageRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.GetImStatuspageRule(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page rule failed", err.Error())
		return
	}
	state.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imStatuspageRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imStatuspageRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.TeamID.Equal(state.TeamID) {
		patch["teamId"] = plan.TeamID.ValueInt64()
	}
	if !plan.StatusPageID.Equal(state.StatusPageID) {
		patch["statusPageId"] = plan.StatusPageID.ValueInt64()
	}
	if !plan.MinSeverity.Equal(state.MinSeverity) {
		patch["minSeverity"] = plan.MinSeverity.ValueString()
	}
	if !plan.TargetState.Equal(state.TargetState) {
		patch["targetState"] = plan.TargetState.ValueString()
	}
	if !plan.AutoResolve.Equal(state.AutoResolve) {
		patch["autoResolve"] = plan.AutoResolve.ValueBool()
	}
	rule, err := r.client.UpdateImStatuspageRule(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating status page rule failed", err.Error())
		return
	}
	plan.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imStatuspageRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imStatuspageRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteImStatuspageRule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page rule failed", err.Error())
	}
}

func (r *imStatuspageRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---- data source -------------------------------------------------------------------------------

type imStatuspageRuleDataSource struct{ client *client.Client }

func NewImStatuspageRuleDataSource() datasource.DataSource { return &imStatuspageRuleDataSource{} }

func (d *imStatuspageRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_statuspage_rule"
}

func (d *imStatuspageRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "Look up one Incident Management status page rule by numeric id.",
		Attributes: map[string]dschema.Attribute{
			"id":              dschema.StringAttribute{Required: true, Description: "Numeric rule id."},
			"organization_id": dschema.Int64Attribute{Computed: true},
			"team_id":         dschema.Int64Attribute{Computed: true},
			"status_page_id":  dschema.Int64Attribute{Computed: true},
			"min_severity":    dschema.StringAttribute{Computed: true},
			"target_state":    dschema.StringAttribute{Computed: true},
			"auto_resolve":    dschema.BoolAttribute{Computed: true},
			"created_at":      dschema.StringAttribute{Computed: true},
			"updated_at":      dschema.StringAttribute{Computed: true},
		},
	}
}

func (d *imStatuspageRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imStatuspageRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imStatuspageRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := d.client.GetImStatuspageRule(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading status page rule failed", err.Error())
		return
	}
	cfg.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
