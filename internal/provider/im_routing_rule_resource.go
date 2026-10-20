package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &imRoutingRuleResource{}
	_ resource.ResourceWithImportState = &imRoutingRuleResource{}
)

type imRoutingRuleResource struct{ client *client.Client }

type routingMatchModel struct {
	SourceIDs    []int64           `tfsdk:"source_ids"`
	CustomerIDs  []int64           `tfsdk:"customer_ids"`
	MonitorIDs   []int64           `tfsdk:"monitor_ids"`
	Tags         []string          `tfsdk:"tags"`
	Severity     []string          `tfsdk:"severity"`
	FieldMatches map[string]string `tfsdk:"field_matches"`
}

// imRoutingRuleModel is shared by the uptimeify_im_routing_rule resource and data source.
type imRoutingRuleModel struct {
	ID               types.String       `tfsdk:"id"`
	OrganizationID   types.Int64        `tfsdk:"organization_id"`
	TeamID           types.Int64        `tfsdk:"team_id"`
	Priority         types.Int64        `tfsdk:"priority"`
	Match            *routingMatchModel `tfsdk:"match"`
	SeverityOverride types.String       `tfsdk:"severity_override"`
	CreatedAt        types.String       `tfsdk:"created_at"`
	UpdatedAt        types.String       `tfsdk:"updated_at"`
}

func (m *routingMatchModel) toAPI() client.RoutingMatch {
	return client.RoutingMatch{SourceIDs: ptrSlice(m.SourceIDs), CustomerIDs: ptrSlice(m.CustomerIDs),
		MonitorIDs: ptrSlice(m.MonitorIDs), Tags: ptrSlice(m.Tags), Severity: ptrSlice(m.Severity), FieldMatches: ptrMap(m.FieldMatches)}
}

// fromAPI fills m. An empty stored match ({} = catch-all) stays null unless match = {} was written.
func (m *imRoutingRuleModel) fromAPI(r *client.RoutingRule) {
	m.ID = types.StringValue(strconv.FormatInt(r.ID, 10))
	m.OrganizationID = types.Int64Value(r.OrganizationID)
	m.TeamID = types.Int64Value(r.TeamID)
	m.Priority = types.Int64Value(r.Priority)
	m.SeverityOverride = types.StringPointerValue(r.SeverityOverride)
	m.CreatedAt = types.StringValue(r.CreatedAt)
	m.UpdatedAt = types.StringValue(r.UpdatedAt)
	var api client.RoutingMatch
	if r.Match != nil {
		api = *r.Match
	}
	match := &routingMatchModel{SourceIDs: derefSlice(api.SourceIDs), CustomerIDs: derefSlice(api.CustomerIDs),
		MonitorIDs: derefSlice(api.MonitorIDs), Tags: derefSlice(api.Tags), Severity: derefSlice(api.Severity),
		FieldMatches: derefMap(api.FieldMatches)}
	empty := match.SourceIDs == nil && match.CustomerIDs == nil && match.MonitorIDs == nil && match.Tags == nil &&
		match.Severity == nil && match.FieldMatches == nil
	if !empty || m.Match != nil {
		m.Match = match
	}
}

func NewImRoutingRuleResource() resource.Resource { return &imRoutingRuleResource{} }

func (r *imRoutingRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_routing_rule"
}

func (r *imRoutingRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	ids := func(d string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.Int64Type, Optional: true, Description: d + " At most 500.",
			Validators: []validator.List{listvalidator.SizeAtMost(500), listvalidator.ValueInt64sAre(int64validator.AtLeast(1))}}
	}
	resp.Schema = schema.Schema{
		Description: "An Incident Management routing rule: incidents whose alert matches go to `team_id`. Rules run " +
			"in ascending `priority`. Incident Management must be active for the organization. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric rule id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Required: true, Description: "The team that receives matching incidents.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"priority": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(100),
				Description: "Lower runs first. Default `100`.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"match": schema.SingleNestedAttribute{
				Optional: true,
				Description: "What the rule matches; all set fields must match. Unset or `{}` = catch-all. " +
					"All ids must belong to the organization.",
				Attributes: map[string]schema.Attribute{
					"source_ids":   ids("Alert source ids."),
					"customer_ids": ids("Customer ids."),
					"monitor_ids":  ids("Ids of any monitor type (website, DNS, ICMP, ...)."),
					"tags": schema.ListAttribute{ElementType: types.StringType, Optional: true, Description: "Alert tags. At most 200.",
						Validators: []validator.List{listvalidator.SizeAtMost(200), listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1))}},
					"severity": schema.ListAttribute{ElementType: types.StringType, Optional: true, Description: "`sev1` to `sev4`.",
						Validators: []validator.List{listvalidator.ValueStringsAre(sevValidator...)}},
					"field_matches": schema.MapAttribute{ElementType: types.StringType, Optional: true,
						Description: "Payload field -> expected value, at most 50 entries.",
						Validators:  []validator.Map{mapvalidator.SizeAtMost(50)}},
				},
			},
			"severity_override": schema.StringAttribute{Optional: true, Description: "Unset = keep the mapped severity.",
				Validators: sevValidator},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *imRoutingRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *imRoutingRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imRoutingRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"teamId": plan.TeamID.ValueInt64(), "priority": plan.Priority.ValueInt64()}
	if plan.Match != nil {
		body["match"] = plan.Match.toAPI()
	}
	if !plan.SeverityOverride.IsNull() {
		body["severityOverride"] = plan.SeverityOverride.ValueString()
	}
	rule, err := r.client.CreateRoutingRule(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating routing rule failed", err.Error())
		return
	}
	plan.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imRoutingRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imRoutingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.GetRoutingRule(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading routing rule failed", err.Error())
		return
	}
	state.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imRoutingRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imRoutingRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.TeamID.Equal(state.TeamID) {
		patch["teamId"] = plan.TeamID.ValueInt64()
	}
	if !plan.Priority.Equal(state.Priority) {
		patch["priority"] = plan.Priority.ValueInt64()
	}
	var planMatch, stateMatch *client.RoutingMatch
	if plan.Match != nil {
		m := plan.Match.toAPI()
		planMatch = &m
	}
	if state.Match != nil {
		m := state.Match.toAPI()
		stateMatch = &m
	}
	if !jsonEqual(planMatch, stateMatch) {
		patch["match"] = planMatch // nil = null = catch-all
	}
	if !plan.SeverityOverride.Equal(state.SeverityOverride) {
		patch["severityOverride"] = plan.SeverityOverride.ValueStringPointer()
	}
	var rule *client.RoutingRule
	var err error
	if len(patch) > 0 {
		rule, err = r.client.UpdateRoutingRule(ctx, state.ID.ValueString(), patch)
	} else {
		rule, err = r.client.GetRoutingRule(ctx, state.ID.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating routing rule failed", err.Error())
		return
	}
	plan.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imRoutingRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imRoutingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRoutingRule(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting routing rule failed", err.Error())
	}
}

func (r *imRoutingRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// data source

var _ datasource.DataSourceWithConfigure = &imRoutingRuleDataSource{}

type imRoutingRuleDataSource struct{ client *client.Client }

func NewImRoutingRuleDataSource() datasource.DataSource { return &imRoutingRuleDataSource{} }

func (d *imRoutingRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_routing_rule"
}

func (d *imRoutingRuleDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&imRoutingRuleResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema)
	attrs["id"] = dschema.StringAttribute{Required: true, Description: "Numeric rule id."}
	resp.Schema = dschema.Schema{Description: "Read one Incident Management routing rule by id.", Attributes: attrs}
}

func (d *imRoutingRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imRoutingRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m imRoutingRuleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := d.client.GetRoutingRule(ctx, m.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading routing rule failed", err.Error())
		return
	}
	m.fromAPI(rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
