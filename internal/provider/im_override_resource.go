package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

// imOverrideResource serves uptimeify_im_schedule_override (parent "schedules") and
// uptimeify_im_team_override (parent "teams"); both share the override model of im-schedules.ts.
type imOverrideResource struct {
	client *client.Client
	parent string // "schedules" or "teams"
}

var (
	_ resource.ResourceWithConfigure   = &imOverrideResource{}
	_ resource.ResourceWithImportState = &imOverrideResource{}
)

func NewIMScheduleOverrideResource() resource.Resource {
	return &imOverrideResource{parent: "schedules"}
}
func NewIMTeamOverrideResource() resource.Resource { return &imOverrideResource{parent: "teams"} }

type imOverrideModel struct {
	ID                  types.String `tfsdk:"id"`
	TeamID              types.Int64  `tfsdk:"team_id"`     // team override only
	ScheduleID          types.Int64  `tfsdk:"schedule_id"` // parent of a schedule override, optional on a team override
	OverrideID          types.Int64  `tfsdk:"override_id"`
	UserID              types.String `tfsdk:"user_id"`
	Type                types.String `tfsdk:"type"`
	ApplyScope          types.String `tfsdk:"apply_scope"`
	EscalationTierOrder types.Int64  `tfsdk:"escalation_tier_order"`
	CoveredByUserIDs    types.Set    `tfsdk:"covered_by_user_ids"`
	StartsAt            types.String `tfsdk:"starts_at"`
	EndsAt              types.String `tfsdk:"ends_at"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

// scheduleOverrideModel is imOverrideModel without team_id (tfsdk needs exact attribute sets).
type scheduleOverrideModel struct {
	ID                  types.String `tfsdk:"id"`
	ScheduleID          types.Int64  `tfsdk:"schedule_id"`
	OverrideID          types.Int64  `tfsdk:"override_id"`
	UserID              types.String `tfsdk:"user_id"`
	Type                types.String `tfsdk:"type"`
	ApplyScope          types.String `tfsdk:"apply_scope"`
	EscalationTierOrder types.Int64  `tfsdk:"escalation_tier_order"`
	CoveredByUserIDs    types.Set    `tfsdk:"covered_by_user_ids"`
	StartsAt            types.String `tfsdk:"starts_at"`
	EndsAt              types.String `tfsdk:"ends_at"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

func (r *imOverrideResource) isTeam() bool { return r.parent == "teams" }

func (m *imOverrideModel) parentID(team bool) int64 {
	if team {
		return m.TeamID.ValueInt64()
	}
	return m.ScheduleID.ValueInt64()
}

func (m *imOverrideModel) fromAPI(team bool, o *client.IMOverride) {
	m.ID = types.StringValue(idPair(m.parentID(team), o.ID))
	m.OverrideID = types.Int64Value(o.ID)
	if team {
		m.ScheduleID = types.Int64PointerValue(o.ScheduleID)
	}
	m.UserID = types.StringValue(o.UserID)
	m.Type = types.StringValue(o.Type)
	m.ApplyScope = types.StringValue(o.ApplyScope)
	m.EscalationTierOrder = types.Int64PointerValue(o.EscalationTierOrder)
	ids := []string{}
	for _, c := range o.CoveredBy {
		ids = append(ids, c.UserID)
	}
	m.CoveredByUserIDs = stringSetValue(ids)
	m.StartsAt = keepSameInstant(m.StartsAt, o.StartsAt)
	m.EndsAt = keepSameInstant(m.EndsAt, o.EndsAt)
	m.CreatedAt = types.StringValue(o.CreatedAt)
}

// body is the full override: the PATCH endpoints take the whole object too.
func (m *imOverrideModel) body(ctx context.Context, team bool) map[string]any {
	covered := setStrings(ctx, m.CoveredByUserIDs)
	sort.Strings(covered)
	if covered == nil {
		covered = []string{}
	}
	b := map[string]any{
		"userId":              m.UserID.ValueString(),
		"type":                m.Type.ValueString(),
		"applyScope":          m.ApplyScope.ValueString(),
		"escalationTierOrder": m.EscalationTierOrder.ValueInt64Pointer(),
		"coveredByUserIds":    covered,
		"startsAt":            m.StartsAt.ValueString(),
		"endsAt":              m.EndsAt.ValueString(),
	}
	if team {
		b["scheduleId"] = m.ScheduleID.ValueInt64Pointer()
	}
	return b
}

func (r *imOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	if r.isTeam() {
		resp.TypeName = req.ProviderTypeName + "_im_team_override"
	} else {
		resp.TypeName = req.ProviderTypeName + "_im_schedule_override"
	}
}

func (r *imOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceInt := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	timeDesc := " RFC3339 timestamp. The API answers in UTC with milliseconds, another spelling of the same instant is kept as written."
	attrs := map[string]schema.Attribute{
		"override_id": schema.Int64Attribute{Computed: true, Description: "Numeric override id.",
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
		"user_id": schema.StringAttribute{Required: true, Description: "User going online or offline; an admin, editor or responder of the organization.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
		"type": schema.StringAttribute{Required: true, Description: "`online` (user takes the shift) or `offline` (user is away).",
			Validators: []validator.String{stringvalidator.OneOf("online", "offline")}},
		"apply_scope": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("this_team"),
			Description: "`this_team` (default) or `all_teams`.", Validators: []validator.String{stringvalidator.OneOf("this_team", "all_teams")}},
		"escalation_tier_order": schema.Int64Attribute{Optional: true, Description: "Only this tier position; unset = all tiers.",
			Validators: []validator.Int64{int64validator.AtLeast(1)}},
		"covered_by_user_ids": schema.SetAttribute{ElementType: types.StringType, Optional: true, Computed: true,
			Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
			Description: "`offline` only: who covers (at most 200). Empty = a gap that auto fill-up backstops.",
			Validators:  []validator.Set{setvalidator.SizeAtMost(200)}},
		"starts_at":  schema.StringAttribute{Required: true, Description: "Start." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
		"ends_at":    schema.StringAttribute{Required: true, Description: "End, after `starts_at`." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
		"created_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
	}
	idAttr := func(desc string) schema.Attribute {
		return schema.StringAttribute{Computed: true, Description: desc, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	}
	if r.isTeam() {
		attrs["id"] = idAttr("`<team_id>/<override_id>`.")
		attrs["team_id"] = schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: replaceInt}
		attrs["schedule_id"] = schema.Int64Attribute{Optional: true, Description: "A schedule of this team; unset = team-level override.",
			Validators: []validator.Int64{int64validator.AtLeast(1)}}
		resp.Schema = schema.Schema{
			Description: "An override (vacation, swap) owned by an Incident Management team, optionally on one of its schedules. Import with `<team_id>/<override_id>`.",
			Attributes:  attrs,
		}
		return
	}
	attrs["id"] = idAttr("`<schedule_id>/<override_id>`.")
	attrs["schedule_id"] = schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: replaceInt}
	resp.Schema = schema.Schema{
		Description: "An override (vacation, swap) on an on-call schedule. Import with `<schedule_id>/<override_id>`.",
		Attributes:  attrs,
	}
}

func (r *imOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// getModel/toValue convert between the framework data and imOverrideModel (the schedule variant has no team_id).
func (r *imOverrideResource) getModel(ctx context.Context, get func(context.Context, any) diag.Diagnostics, m *imOverrideModel) diag.Diagnostics {
	if r.isTeam() {
		return get(ctx, m)
	}
	var s scheduleOverrideModel
	d := get(ctx, &s)
	*m = imOverrideModel{ID: s.ID, TeamID: types.Int64Null(), ScheduleID: s.ScheduleID, OverrideID: s.OverrideID, UserID: s.UserID, Type: s.Type,
		ApplyScope: s.ApplyScope, EscalationTierOrder: s.EscalationTierOrder, CoveredByUserIDs: s.CoveredByUserIDs,
		StartsAt: s.StartsAt, EndsAt: s.EndsAt, CreatedAt: s.CreatedAt}
	return d
}

func (r *imOverrideResource) toValue(m *imOverrideModel) any {
	if r.isTeam() {
		return m
	}
	return &scheduleOverrideModel{ID: m.ID, ScheduleID: m.ScheduleID, OverrideID: m.OverrideID, UserID: m.UserID, Type: m.Type,
		ApplyScope: m.ApplyScope, EscalationTierOrder: m.EscalationTierOrder, CoveredByUserIDs: m.CoveredByUserIDs,
		StartsAt: m.StartsAt, EndsAt: m.EndsAt, CreatedAt: m.CreatedAt}
}

func (r *imOverrideResource) parentPath(m *imOverrideModel) string {
	return client.OverrideParent(r.parent, m.parentID(r.isTeam()))
}

func (r *imOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imOverrideModel
	resp.Diagnostics.Append(r.getModel(ctx, req.Plan.Get, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.CreateIMOverride(ctx, r.parentPath(&plan), plan.body(ctx, r.isTeam()))
	if err != nil {
		resp.Diagnostics.AddError("Creating override failed", err.Error())
		return
	}
	o, err := r.client.GetIMOverride(ctx, r.parentPath(&plan), id)
	if err != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), idPair(plan.parentID(r.isTeam()), id))...)
		resp.Diagnostics.AddError("Reading override failed", err.Error())
		return
	}
	plan.fromAPI(r.isTeam(), o)
	resp.Diagnostics.Append(resp.State.Set(ctx, r.toValue(&plan))...)
}

func (r *imOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imOverrideModel
	resp.Diagnostics.Append(r.getModel(ctx, req.State.Get, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetIMOverride(ctx, r.parentPath(&state), state.OverrideID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading override failed", err.Error())
		return
	}
	state.fromAPI(r.isTeam(), o)
	resp.Diagnostics.Append(resp.State.Set(ctx, r.toValue(&state))...)
}

func (r *imOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imOverrideModel
	resp.Diagnostics.Append(r.getModel(ctx, req.Plan.Get, &plan)...)
	resp.Diagnostics.Append(r.getModel(ctx, req.State.Get, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.OverrideID.ValueInt64()
	if err := r.client.UpdateIMOverride(ctx, r.parentPath(&state), id, plan.body(ctx, r.isTeam())); err != nil {
		resp.Diagnostics.AddError("Updating override failed", err.Error())
		return
	}
	o, err := r.client.GetIMOverride(ctx, r.parentPath(&state), id)
	if err != nil {
		resp.Diagnostics.AddError("Reading override failed", err.Error())
		return
	}
	plan.fromAPI(r.isTeam(), o)
	resp.Diagnostics.Append(resp.State.Set(ctx, r.toValue(&plan))...)
}

func (r *imOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imOverrideModel
	resp.Diagnostics.Append(r.getModel(ctx, req.State.Get, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMOverride(ctx, r.parentPath(&state), state.OverrideID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting override failed", err.Error())
	}
}

func (r *imOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parentAttr := "schedule_id"
	if r.isTeam() {
		parentAttr = "team_id"
	}
	parentID, overrideID, err := parseTwoIDs(req.ID, "<"+parentAttr+">/<override_id>")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(parentAttr), parentID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("override_id"), overrideID)...)
}
