package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
	_ resource.ResourceWithConfigure   = &imTierResource{}
	_ resource.ResourceWithImportState = &imTierResource{}
)

type imTierResource struct{ client *client.Client }

type imTierModel struct {
	ID                         types.String        `tfsdk:"id"`
	TeamID                     types.Int64         `tfsdk:"team_id"`
	TierID                     types.Int64         `tfsdk:"tier_id"`
	TierOrder                  types.Int64         `tfsdk:"tier_order"`
	MemberUserIDs              []types.String      `tfsdk:"member_user_ids"`
	DefaultScheduleID          types.Int64         `tfsdk:"default_schedule_id"`
	AutoEscalationEnabled      types.Bool          `tfsdk:"auto_escalation_enabled"`
	AutoEscalationAfterMinutes types.Int64         `tfsdk:"auto_escalation_after_minutes"`
	AutoEscalationStopMode     types.String        `tfsdk:"auto_escalation_stop_mode"`
	AutoEscalationSeverities   types.Set           `tfsdk:"auto_escalation_severities"`
	AutoEscalationTimeFilters  []weeklyWindowModel `tfsdk:"auto_escalation_time_filters"`
	Repeats                    types.Int64         `tfsdk:"repeats"`
	RepeatsAfterMinutes        types.Int64         `tfsdk:"repeats_after_minutes"`
	RepeatsStopMode            types.String        `tfsdk:"repeats_stop_mode"`
}

// fromAPI fills m. The default schedule is the one the API created with the tier; on import
// (unknown id) it is the tier's oldest schedule. member_user_ids is its first rotation.
func (m *imTierModel) fromAPI(teamID int64, t *client.IMTier) {
	m.ID = types.StringValue(idPair(teamID, t.ID))
	m.TeamID = types.Int64Value(teamID)
	m.TierID = types.Int64Value(t.ID)
	m.TierOrder = types.Int64Value(t.TierOrder)
	m.AutoEscalationEnabled = types.BoolValue(t.AutoEscalationEnabled)
	m.AutoEscalationAfterMinutes = types.Int64Value(t.AutoEscalationAfterMinutes)
	m.AutoEscalationStopMode = types.StringValue(t.AutoEscalationStopMode)
	m.AutoEscalationSeverities = types.SetNull(types.StringType)
	if t.AutoEscalationSeverities != nil {
		m.AutoEscalationSeverities = stringSetValue(t.AutoEscalationSeverities)
	}
	m.AutoEscalationTimeFilters = weeklyFromAPI(t.AutoEscalationTimeFilters)
	m.Repeats = types.Int64Value(t.Repeats)
	m.RepeatsAfterMinutes = types.Int64PointerValue(t.RepeatsAfterMinutes)
	m.RepeatsStopMode = types.StringValue(t.RepeatsStopMode)

	if (m.DefaultScheduleID.IsNull() || m.DefaultScheduleID.IsUnknown()) && len(t.Schedules) > 0 {
		m.DefaultScheduleID = types.Int64Value(t.Schedules[0].ID)
	}
	for _, s := range t.Schedules {
		if s.ID != m.DefaultScheduleID.ValueInt64() {
			continue
		}
		for _, rot := range s.Rotations {
			if rot.RotationOrder == 1 {
				m.MemberUserIDs = nil
				for _, mem := range rot.Members {
					m.MemberUserIDs = append(m.MemberUserIDs, types.StringValue(mem.UserID))
				}
			}
		}
	}
}

func (m *imTierModel) memberIDs() []string {
	out := make([]string, 0, len(m.MemberUserIDs))
	for _, v := range m.MemberUserIDs {
		out = append(out, v.ValueString())
	}
	return out
}

func imTierPatch(ctx context.Context, plan, cur *imTierModel) map[string]any {
	p := map[string]any{}
	if !plan.AutoEscalationEnabled.Equal(cur.AutoEscalationEnabled) {
		p["autoEscalationEnabled"] = plan.AutoEscalationEnabled.ValueBool()
	}
	if !plan.AutoEscalationAfterMinutes.Equal(cur.AutoEscalationAfterMinutes) {
		p["autoEscalationAfterMinutes"] = plan.AutoEscalationAfterMinutes.ValueInt64()
	}
	if !plan.AutoEscalationStopMode.Equal(cur.AutoEscalationStopMode) {
		p["autoEscalationStopMode"] = plan.AutoEscalationStopMode.ValueString()
	}
	if !plan.AutoEscalationSeverities.Equal(cur.AutoEscalationSeverities) {
		p["autoEscalationSeverities"] = setStrings(ctx, plan.AutoEscalationSeverities) // nil = all
	}
	if !sameWeekly(plan.AutoEscalationTimeFilters, cur.AutoEscalationTimeFilters) {
		p["autoEscalationTimeFilters"] = weeklyToAPI(plan.AutoEscalationTimeFilters)
	}
	if !plan.Repeats.Equal(cur.Repeats) {
		p["repeats"] = plan.Repeats.ValueInt64()
	}
	if !plan.RepeatsAfterMinutes.Equal(cur.RepeatsAfterMinutes) {
		p["repeatsAfterMinutes"] = plan.RepeatsAfterMinutes.ValueInt64Pointer()
	}
	if !plan.RepeatsStopMode.Equal(cur.RepeatsStopMode) {
		p["repeatsStopMode"] = plan.RepeatsStopMode.ValueString()
	}
	return p
}

func NewIMEscalationTierResource() resource.Resource { return &imTierResource{} }

func (r *imTierResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_escalation_tier"
}

func (r *imTierResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An escalation tier of an Incident Management team. Creating a tier also creates its default schedule " +
			"(`default_schedule_id`, rotation mode `none`) whose single rotation is `member_user_ids`; further schedules of the tier " +
			"are `uptimeify_im_schedule`. Import with `<team_id>/<tier_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "`<team_id>/<tier_id>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"tier_id": schema.Int64Attribute{Computed: true, Description: "Numeric tier id.", PlanModifiers: keepInt},
			"tier_order": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepInt,
				Description: "Position in the team's ladder, 1 = paged first. Unset = appended at the end. On create the tier is inserted there " +
					"(tiers at or after it move down by one); a later change reorders the team's tiers (POST .../tiers/reorder). Both move the " +
					"other tiers too: when you set it, set it on every tier of the team, as 1..n. A value larger than the number of tiers ends up as the last position.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"member_user_ids": schema.ListAttribute{ElementType: types.StringType, Required: true,
				Description: "Team members on the default schedule's rotation, in order (1 to 200 user ids). Changing it replaces all rotation groups of the default schedule.",
				Validators:  []validator.List{listvalidator.SizeBetween(1, 200), listvalidator.UniqueValues()}},
			"default_schedule_id": schema.Int64Attribute{Computed: true, PlanModifiers: keepInt,
				Description: "Schedule the API created with the tier. Do not also manage it as `uptimeify_im_schedule`."},
			"auto_escalation_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Default `true`."},
			"auto_escalation_after_minutes": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(5),
				Description: "Minutes before escalating to the next tier. Default `5`.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"auto_escalation_stop_mode": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("acknowledged"),
				Description: "`acknowledged` or `resolved`. Default `acknowledged`.", Validators: []validator.String{stringvalidator.OneOf(imStopModes...)}},
			"auto_escalation_severities": schema.SetAttribute{ElementType: types.StringType, Optional: true,
				Description: "Severities that escalate: `critical`, `warning`, `minor`. Unset = all.",
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1), setvalidator.ValueStringsAre(stringvalidator.OneOf("critical", "warning", "minor"))}},
			"auto_escalation_time_filters": weeklyWindowsAttribute("Weekly windows in which auto-escalation runs. Empty (default) = always."),
			"repeats": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Description: "Extra pages on this tier before moving on. Default `0`.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"repeats_after_minutes": schema.Int64Attribute{Optional: true, Description: "Minutes between repeats; unset = `auto_escalation_after_minutes`.",
				Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"repeats_stop_mode": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("acknowledged"),
				Description: "`acknowledged` or `resolved`. Default `acknowledged`.", Validators: []validator.String{stringvalidator.OneOf(imStopModes...)}},
		},
	}
}

func (r *imTierResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// moveTier puts tierID at position want (1-based, clamped) via reorder, if it is not there already.
func (r *imTierResource) moveTier(ctx context.Context, teamID, tierID, want int64) error {
	tiers, err := r.client.ListIMTiers(ctx, teamID)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tiers))
	cur := -1
	for i, t := range tiers {
		ids = append(ids, t.ID)
		if t.ID == tierID {
			cur = i
		}
	}
	if cur < 0 {
		return fmt.Errorf("tier %d is not in team %d", tierID, teamID)
	}
	pos := int(min(want, int64(len(ids)))) - 1
	if pos == cur && tiers[cur].TierOrder == want {
		return nil
	}
	ids = slices.Delete(ids, cur, cur+1)
	ids = slices.Insert(ids, pos, tierID)
	return r.client.ReorderIMTiers(ctx, teamID, ids)
}

func (r *imTierResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imTierModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	teamID := plan.TeamID.ValueInt64()

	var tierOrder *int64
	if !plan.TierOrder.IsUnknown() && !plan.TierOrder.IsNull() {
		tierOrder = plan.TierOrder.ValueInt64Pointer()
	}
	// Two tiers created at once race for the next tierOrder; the API answers 409 imTierOrderConflict.
	// A tierOrder past the end (422 imInvalidNumber, e.g. tier 3 created before tier 2) is appended:
	// the last position, as documented.
	// All configured settings go into the one POST: diffing the plan against an empty model sends
	// every set value and leaves unset ones (severities, repeats_after_minutes) to the API default.
	settings := imTierPatch(ctx, &plan, &imTierModel{AutoEscalationSeverities: types.SetNull(types.StringType)})
	var tier *client.IMTier
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		tier, err = r.client.CreateIMTier(ctx, teamID, plan.memberIDs(), tierOrder, settings)
		var apiErr *client.APIError
		if !errors.As(err, &apiErr) {
			break
		}
		if apiErr.Code == "imInvalidNumber" && tierOrder != nil {
			tierOrder = nil
			continue
		}
		if apiErr.Code != "imTierOrderConflict" {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	if err != nil {
		resp.Diagnostics.AddError("Creating escalation tier failed", err.Error())
		return
	}
	cur := imTierModel{DefaultScheduleID: types.Int64Null()}
	cur.fromAPI(teamID, tier)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cur)...)
}

func (r *imTierResource) readInto(ctx context.Context, m *imTierModel, addError func(string, string)) bool {
	t, err := r.client.GetIMTier(ctx, m.TeamID.ValueInt64(), m.TierID.ValueInt64())
	if err != nil {
		addError("Reading escalation tier failed", err.Error())
		return false
	}
	m.fromAPI(m.TeamID.ValueInt64(), t)
	return true
}

func (r *imTierResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imTierModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetIMTier(ctx, state.TeamID.ValueInt64(), state.TierID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading escalation tier failed", err.Error())
		return
	}
	state.fromAPI(state.TeamID.ValueInt64(), t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imTierResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imTierModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	teamID, tierID := state.TeamID.ValueInt64(), state.TierID.ValueInt64()
	if p := imTierPatch(ctx, &plan, &state); len(p) > 0 {
		if err := r.client.UpdateIMTier(ctx, teamID, tierID, p); err != nil {
			resp.Diagnostics.AddError("Updating escalation tier failed", err.Error())
			return
		}
	}
	if !slices.Equal(plan.memberIDs(), state.memberIDs()) {
		if state.DefaultScheduleID.IsNull() {
			resp.Diagnostics.AddError("Updating tier members failed", "The tier has no default schedule any more.")
			return
		}
		body := map[string]any{"rotations": []map[string]any{{"members": plan.memberIDs()}}}
		if err := r.client.UpdateIMSchedule(ctx, state.DefaultScheduleID.ValueInt64(), body); err != nil {
			resp.Diagnostics.AddError("Updating tier members failed", err.Error())
			return
		}
	}
	if !plan.TierOrder.IsUnknown() && !plan.TierOrder.Equal(state.TierOrder) {
		if err := r.moveTier(ctx, teamID, tierID, plan.TierOrder.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("Reordering escalation tiers failed", err.Error())
			return
		}
	}
	state.MemberUserIDs = plan.MemberUserIDs
	if r.readInto(ctx, &state, resp.Diagnostics.AddError) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *imTierResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imTierModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMTier(ctx, state.TeamID.ValueInt64(), state.TierID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting escalation tier failed", err.Error())
	}
}

func (r *imTierResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	teamID, tierID, err := parseTwoIDs(req.ID, "<team_id>/<tier_id>")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), teamID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tier_id"), tierID)...)
}
