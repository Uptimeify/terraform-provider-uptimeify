package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &imScheduleResource{}
	_ resource.ResourceWithImportState = &imScheduleResource{}
)

type imScheduleResource struct{ client *client.Client }

type rotationModel struct {
	Members []types.String `tfsdk:"members"`
}

type imScheduleModel struct {
	ID                  types.String        `tfsdk:"id"`
	TierID              types.Int64         `tfsdk:"tier_id"`
	TeamID              types.Int64         `tfsdk:"team_id"`
	Name                types.String        `tfsdk:"name"`
	Timezone            types.String        `tfsdk:"timezone"`
	EffectiveFrom       types.String        `tfsdk:"effective_from"`
	EffectiveUntil      types.String        `tfsdk:"effective_until"`
	WeeklySchedules     []weeklyWindowModel `tfsdk:"weekly_schedules"`
	RotationMode        types.String        `tfsdk:"rotation_mode"`
	RotationRepeats     types.String        `tfsdk:"rotation_repeats"`
	CustomRepeatUnit    types.String        `tfsdk:"custom_repeat_unit"`
	CustomRepeatValue   types.Int64         `tfsdk:"custom_repeat_value"`
	AutoRotationSize    types.Int64         `tfsdk:"auto_rotation_size"`
	RoundRobinSize      types.Int64         `tfsdk:"round_robin_size"`
	StartsOnDayOfWeek   types.Int64         `tfsdk:"starts_on_day_of_week"`
	StartsOnDateOfMonth types.Int64         `tfsdk:"starts_on_date_of_month"`
	StartsOnTime        types.String        `tfsdk:"starts_on_time"`
	Rotations           []rotationModel     `tfsdk:"rotations"`
}

func (m *imScheduleModel) numID() int64 {
	n, _ := strconv.ParseInt(m.ID.ValueString(), 10, 64)
	return n
}

func keepSameInstantPtr(prior types.String, api *string) types.String {
	if api == nil {
		return types.StringNull()
	}
	return keepSameInstant(prior, *api)
}

// fromAPI fills m from GET /api/im/schedules/{id} plus the rotations from the tier tree.
func (m *imScheduleModel) fromAPI(s *client.IMSchedule, rotations []client.IMRotation) {
	m.ID = types.StringValue(strconv.FormatInt(s.ID, 10))
	m.TierID = types.Int64Value(s.TierID)
	m.TeamID = types.Int64Value(s.TeamID)
	m.Name = types.StringValue(s.Name)
	m.Timezone = types.StringValue(s.Timezone)
	m.EffectiveFrom = keepSameInstantPtr(m.EffectiveFrom, s.EffectiveFrom)
	m.EffectiveUntil = keepSameInstantPtr(m.EffectiveUntil, s.EffectiveUntil)
	m.WeeklySchedules = weeklyFromAPI(s.WeeklySchedules)
	m.RotationMode = types.StringValue(s.RotationMode)
	m.RotationRepeats = types.StringPointerValue(s.RotationRepeats)
	m.CustomRepeatUnit = types.StringPointerValue(s.CustomRepeatUnit)
	m.CustomRepeatValue = types.Int64PointerValue(s.CustomRepeatValue)
	m.AutoRotationSize = types.Int64PointerValue(s.AutoRotationSize)
	m.RoundRobinSize = types.Int64Value(s.RoundRobinSize)
	m.StartsOnDayOfWeek = types.Int64PointerValue(s.StartsOnDayOfWeek)
	m.StartsOnDateOfMonth = types.Int64PointerValue(s.StartsOnDateOfMonth)
	m.StartsOnTime = types.StringPointerValue(s.StartsOnTime)
	m.Rotations = []rotationModel{}
	for _, r := range rotations { // ordered by rotationOrder, members by memberOrder
		rm := rotationModel{Members: []types.String{}}
		for _, mem := range r.Members {
			rm.Members = append(rm.Members, types.StringValue(mem.UserID))
		}
		m.Rotations = append(m.Rotations, rm)
	}
}

func rotationsToAPI(in []rotationModel) []map[string]any {
	out := []map[string]any{}
	for _, r := range in {
		members := []string{}
		for _, v := range r.Members {
			members = append(members, v.ValueString())
		}
		out = append(out, map[string]any{"members": members})
	}
	return out
}

func sameRotations(a, b []rotationModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].Members) != len(b[i].Members) {
			return false
		}
		for j := range a[i].Members {
			if !a[i].Members[j].Equal(b[i].Members[j]) {
				return false
			}
		}
	}
	return true
}

// imSchedulePatch holds the fields where plan differs from cur; with cur = nil it is the full create body.
func imSchedulePatch(plan, cur *imScheduleModel) map[string]any {
	p := map[string]any{}
	str := func(key string, a types.String, b func(*imScheduleModel) types.String) {
		if cur == nil || !a.Equal(b(cur)) {
			p[key] = a.ValueStringPointer()
		}
	}
	num := func(key string, a types.Int64, b func(*imScheduleModel) types.Int64) {
		if cur == nil || !a.Equal(b(cur)) {
			p[key] = a.ValueInt64Pointer()
		}
	}
	str("name", plan.Name, func(m *imScheduleModel) types.String { return m.Name })
	str("timezone", plan.Timezone, func(m *imScheduleModel) types.String { return m.Timezone })
	str("effectiveFrom", plan.EffectiveFrom, func(m *imScheduleModel) types.String { return m.EffectiveFrom })
	str("effectiveUntil", plan.EffectiveUntil, func(m *imScheduleModel) types.String { return m.EffectiveUntil })
	str("rotationMode", plan.RotationMode, func(m *imScheduleModel) types.String { return m.RotationMode })
	str("rotationRepeats", plan.RotationRepeats, func(m *imScheduleModel) types.String { return m.RotationRepeats })
	str("customRepeatUnit", plan.CustomRepeatUnit, func(m *imScheduleModel) types.String { return m.CustomRepeatUnit })
	num("customRepeatValue", plan.CustomRepeatValue, func(m *imScheduleModel) types.Int64 { return m.CustomRepeatValue })
	num("autoRotationSize", plan.AutoRotationSize, func(m *imScheduleModel) types.Int64 { return m.AutoRotationSize })
	num("roundRobinSize", plan.RoundRobinSize, func(m *imScheduleModel) types.Int64 { return m.RoundRobinSize })
	num("startsOnDayOfWeek", plan.StartsOnDayOfWeek, func(m *imScheduleModel) types.Int64 { return m.StartsOnDayOfWeek })
	num("startsOnDateOfMonth", plan.StartsOnDateOfMonth, func(m *imScheduleModel) types.Int64 { return m.StartsOnDateOfMonth })
	str("startsOnTime", plan.StartsOnTime, func(m *imScheduleModel) types.String { return m.StartsOnTime })
	if cur == nil || !sameWeekly(plan.WeeklySchedules, cur.WeeklySchedules) {
		p["weeklySchedules"] = weeklyToAPI(plan.WeeklySchedules)
	}
	if cur == nil || !sameRotations(plan.Rotations, cur.Rotations) {
		p["rotations"] = rotationsToAPI(plan.Rotations)
	}
	return p
}

func NewIMScheduleResource() resource.Resource { return &imScheduleResource{} }

func (r *imScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_schedule"
}

func (r *imScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	timeDesc := " RFC3339 timestamp; unset = unbounded. The API answers in UTC with milliseconds, another spelling of the same instant is kept as written."
	rotationsDefault := types.ListValueMust(types.ObjectType{AttrTypes: map[string]attr.Type{"members": types.ListType{ElemType: types.StringType}}}, nil)
	resp.Schema = schema.Schema{
		Description: "An on-call schedule on an escalation tier (created via `POST /api/im/tiers/{tierId}/schedules`). " +
			"Rotation members must be members of the tier's team. The tier's own default schedule belongs to `uptimeify_im_escalation_tier`. " +
			"Import with the numeric schedule id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "Numeric schedule id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tier_id": schema.Int64Attribute{Required: true, Description: "Escalation tier (`uptimeify_im_escalation_tier.x.tier_id`). Changing it replaces the schedule.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"team_id": schema.Int64Attribute{Computed: true, Description: "Team of the tier.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "1 to 120 characters.", Validators: []validator.String{stringvalidator.LengthBetween(1, 120)}},
			"timezone": schema.StringAttribute{Required: true, Description: "IANA zone the handovers and weekly windows are read in, e.g. `Europe/Berlin`.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"effective_from":   schema.StringAttribute{Optional: true, Description: "Start of validity." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
			"effective_until":  schema.StringAttribute{Optional: true, Description: "End of validity." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
			"weekly_schedules": weeklyWindowsAttribute("Weekly windows the schedule covers. Empty (default) = around the clock."),
			"rotation_mode": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("none"),
				Description: "`none` (default), `auto` or `explicit`. `auto` and `explicit` need `rotation_repeats` and `starts_on_time`.",
				Validators:  []validator.String{stringvalidator.OneOf("none", "auto", "explicit")}},
			"rotation_repeats": schema.StringAttribute{Optional: true, Description: "`daily`, `weekly`, `biweekly`, `monthly` or `custom`; must be unset for mode `none`.",
				Validators: []validator.String{stringvalidator.OneOf("daily", "weekly", "biweekly", "monthly", "custom")}},
			"custom_repeat_unit": schema.StringAttribute{Optional: true, Description: "For `custom`: `minutes`, `hours`, `days`, `weeks` or `months`.",
				Validators: []validator.String{stringvalidator.OneOf("minutes", "hours", "days", "weeks", "months")}},
			"custom_repeat_value": schema.Int64Attribute{Optional: true, Description: "For `custom`: how many units.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"auto_rotation_size":  schema.Int64Attribute{Optional: true, Description: "For mode `auto`: members per generated rotation group.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"round_robin_size": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1),
				Description: "Members on call at the same time. Default `1`.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"starts_on_day_of_week": schema.Int64Attribute{Optional: true, Description: "Handover weekday (ISO 1-7) for weekly, biweekly and custom-weeks rotations.",
				Validators: []validator.Int64{isoWeekday}},
			"starts_on_date_of_month": schema.Int64Attribute{Optional: true, Description: "Handover day 1-31 for monthly and custom-months rotations.",
				Validators: []validator.Int64{int64validator.Between(1, 31)}},
			"starts_on_time": schema.StringAttribute{Optional: true, Description: "Handover time `HH:mm`; required unless mode `none`.", Validators: []validator.String{hhmm}},
			"rotations": schema.ListNestedAttribute{
				Optional: true, Computed: true, Default: listdefault.StaticValue(rotationsDefault),
				Description: "Ordered rotation groups (at most 50). Every member must be a member of the team.",
				Validators:  []validator.List{listvalidator.SizeAtMost(50)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"members": schema.ListAttribute{ElementType: types.StringType, Required: true, Description: "User ids in order, at most 200, no duplicates.",
						Validators: []validator.List{listvalidator.SizeAtMost(200), listvalidator.UniqueValues()}},
				}},
			},
		},
	}
}

func (r *imScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// readInto loads the schedule and its rotations; notFound reports a schedule (or tier) that is gone.
func (r *imScheduleResource) readInto(ctx context.Context, id int64, m *imScheduleModel) (notFound bool, err error) {
	s, err := r.client.GetIMSchedule(ctx, id)
	if err != nil {
		return client.IsNotFound(err), err
	}
	// GET /api/im/schedules/{id} has no rotations; the tier tree has.
	tier, err := r.client.GetIMTier(ctx, s.TeamID, s.TierID)
	if err != nil {
		return client.IsNotFound(err), err
	}
	var rotations []client.IMRotation
	for _, ts := range tier.Schedules {
		if ts.ID == s.ID {
			rotations = ts.Rotations
		}
	}
	m.fromAPI(s, rotations)
	return false, nil
}

func (r *imScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.CreateIMTierSchedule(ctx, plan.TierID.ValueInt64(), imSchedulePatch(&plan, nil))
	if err != nil {
		resp.Diagnostics.AddError("Creating schedule failed", err.Error())
		return
	}
	if _, err := r.readInto(ctx, id, &plan); err != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(id, 10))...)
		resp.Diagnostics.AddError("Reading schedule failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notFound, err := r.readInto(ctx, state.numID(), &state)
	if notFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading schedule failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if p := imSchedulePatch(&plan, &state); len(p) > 0 {
		if err := r.client.UpdateIMSchedule(ctx, state.numID(), p); err != nil {
			resp.Diagnostics.AddError("Updating schedule failed", err.Error())
			return
		}
	}
	if _, err := r.readInto(ctx, state.numID(), &plan); err != nil {
		resp.Diagnostics.AddError("Reading schedule failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMSchedule(ctx, state.numID()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting schedule failed", err.Error())
	}
}

func (r *imScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if n, err := strconv.ParseInt(strings.TrimSpace(req.ID), 10, 64); err != nil || n < 1 {
		resp.Diagnostics.AddError("Invalid import id", "Expected the numeric schedule id, got "+strconv.Quote(req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
