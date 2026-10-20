package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.ResourceWithConfigure   = &imTeamResource{}
	_ resource.ResourceWithImportState = &imTeamResource{}
)

type imTeamResource struct{ client *client.Client }

// imTeamModel is shared by the uptimeify_im_team resource and data source.
type imTeamModel struct {
	ID                            types.String `tfsdk:"id"`
	Name                          types.String `tfsdk:"name"`
	LabelColor                    types.String `tfsdk:"label_color"`
	AnalyticsEnabled              types.Bool   `tfsdk:"analytics_enabled"`
	Timezone                      types.String `tfsdk:"timezone"`
	EscalationRepeats             types.Int64  `tfsdk:"escalation_repeats"`
	EscalationRepeatsAfterMinutes types.Int64  `tfsdk:"escalation_repeats_after_minutes"`
	EscalationRepeatsStopMode     types.String `tfsdk:"escalation_repeats_stop_mode"`
	EngagementReportEnabled       types.Bool   `tfsdk:"engagement_report_enabled"`
	EngagementReportDayOfWeek     types.Int64  `tfsdk:"engagement_report_day_of_week"`
	EngagementReportTime          types.String `tfsdk:"engagement_report_time"`
	OrganizationID                types.Int64  `tfsdk:"organization_id"`
	CreatedAt                     types.String `tfsdk:"created_at"`
}

func (m *imTeamModel) fromAPI(t *client.IMTeam) {
	m.ID = types.StringValue(strconv.FormatInt(t.ID, 10))
	m.Name = types.StringValue(t.Name)
	m.LabelColor = types.StringPointerValue(t.LabelColor)
	m.AnalyticsEnabled = types.BoolValue(t.AnalyticsEnabled)
	m.Timezone = types.StringPointerValue(t.Timezone)
	m.EscalationRepeats = types.Int64Value(t.EscalationRepeats)
	m.EscalationRepeatsAfterMinutes = types.Int64PointerValue(t.EscalationRepeatsAfterMinutes)
	m.EscalationRepeatsStopMode = types.StringValue(t.EscalationRepeatsStopMode)
	m.EngagementReportEnabled = types.BoolValue(t.EngagementReportEnabled)
	m.EngagementReportDayOfWeek = types.Int64PointerValue(t.EngagementReportDayOfWeek)
	m.EngagementReportTime = types.StringPointerValue(t.EngagementReportTime)
	m.OrganizationID = types.Int64Value(t.OrganizationID)
	m.CreatedAt = types.StringValue(t.CreatedAt)
}

func (m *imTeamModel) numID() int64 { n, _ := strconv.ParseInt(m.ID.ValueString(), 10, 64); return n }

// imTeamPatch holds only the fields where plan differs from cur.
func imTeamPatch(plan, cur *imTeamModel) map[string]any {
	p := map[string]any{}
	if !plan.Name.Equal(cur.Name) {
		p["name"] = plan.Name.ValueString()
	}
	if !plan.LabelColor.Equal(cur.LabelColor) {
		p["labelColor"] = plan.LabelColor.ValueStringPointer()
	}
	if !plan.AnalyticsEnabled.Equal(cur.AnalyticsEnabled) {
		p["analyticsEnabled"] = plan.AnalyticsEnabled.ValueBool()
	}
	if !plan.Timezone.Equal(cur.Timezone) {
		p["timezone"] = plan.Timezone.ValueStringPointer()
	}
	if !plan.EscalationRepeats.Equal(cur.EscalationRepeats) {
		p["escalationRepeats"] = plan.EscalationRepeats.ValueInt64()
	}
	if !plan.EscalationRepeatsAfterMinutes.Equal(cur.EscalationRepeatsAfterMinutes) {
		p["escalationRepeatsAfterMinutes"] = plan.EscalationRepeatsAfterMinutes.ValueInt64Pointer()
	}
	if !plan.EscalationRepeatsStopMode.Equal(cur.EscalationRepeatsStopMode) {
		p["escalationRepeatsStopMode"] = plan.EscalationRepeatsStopMode.ValueString()
	}
	if !plan.EngagementReportEnabled.Equal(cur.EngagementReportEnabled) {
		p["engagementReportEnabled"] = plan.EngagementReportEnabled.ValueBool()
	}
	if !plan.EngagementReportDayOfWeek.Equal(cur.EngagementReportDayOfWeek) {
		p["engagementReportDayOfWeek"] = plan.EngagementReportDayOfWeek.ValueInt64Pointer()
	}
	if !plan.EngagementReportTime.Equal(cur.EngagementReportTime) {
		p["engagementReportTime"] = plan.EngagementReportTime.ValueStringPointer()
	}
	return p
}

func NewIMTeamResource() resource.Resource { return &imTeamResource{} }

func (r *imTeamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_team"
}

func (r *imTeamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An Incident Management team. Needs Incident Management activated for the organization. " +
			"Members, escalation tiers and schedules are separate resources. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric team id."},
			"name": schema.StringAttribute{Required: true, Description: "1 to 120 characters, without surrounding spaces.", Validators: []validator.String{stringvalidator.LengthBetween(1, 120)}},
			"label_color": schema.StringAttribute{Optional: true, Description: "Free-form label color, e.g. `#43B1AE`.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"analytics_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Default `false`."},
			"timezone": schema.StringAttribute{Optional: true, Description: "IANA zone, default for new schedules of the team. Unset = organization default.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"escalation_repeats": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Description: "How often the whole ladder repeats. Default `0`.", Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"escalation_repeats_after_minutes": schema.Int64Attribute{Optional: true, Description: "Minutes before a ladder repeat.",
				Validators: []validator.Int64{int64validator.AtLeast(0)}},
			"escalation_repeats_stop_mode": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("acknowledged"),
				Description: "`acknowledged` or `resolved`. Default `acknowledged`.", Validators: []validator.String{stringvalidator.OneOf(imStopModes...)}},
			"engagement_report_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Default `false`."},
			"engagement_report_day_of_week": schema.Int64Attribute{Optional: true, Description: "ISO weekday 1 (Monday) to 7.",
				Validators: []validator.Int64{isoWeekday}},
			"engagement_report_time": schema.StringAttribute{Optional: true, Description: "`HH:mm`.", Validators: []validator.String{hhmm}},
			"organization_id":        schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"created_at":             schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *imTeamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *imTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imTeamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.CreateIMTeam(ctx, client.IMTeamCreate{
		Name:             plan.Name.ValueString(),
		LabelColor:       plan.LabelColor.ValueStringPointer(),
		AnalyticsEnabled: plan.AnalyticsEnabled.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating IM team failed", err.Error())
		return
	}
	// The settings are not part of POST: read the new row, then patch what differs.
	// A failure from here on leaves the team in state (tainted), not orphaned.
	var cur imTeamModel
	if !r.read(ctx, id, &cur, resp.Diagnostics.AddError) {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(id, 10))...)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cur)...)
	if p := imTeamPatch(&plan, &cur); len(p) > 0 {
		if err := r.client.UpdateIMTeam(ctx, id, p); err != nil {
			resp.Diagnostics.AddError("Setting IM team settings failed", err.Error())
			return
		}
		if !r.read(ctx, id, &cur, resp.Diagnostics.AddError) {
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cur)...)
}

func (r *imTeamResource) read(ctx context.Context, id int64, m *imTeamModel, addError func(string, string)) bool {
	t, err := r.client.GetIMTeam(ctx, id)
	if err != nil {
		addError("Reading IM team failed", err.Error())
		return false
	}
	m.fromAPI(t)
	return true
}

func (r *imTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imTeamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetIMTeam(ctx, state.numID())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading IM team failed", err.Error())
		return
	}
	state.fromAPI(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imTeamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if p := imTeamPatch(&plan, &state); len(p) > 0 {
		if err := r.client.UpdateIMTeam(ctx, state.numID(), p); err != nil {
			resp.Diagnostics.AddError("Updating IM team failed", err.Error())
			return
		}
	}
	if r.read(ctx, state.numID(), &plan, resp.Diagnostics.AddError) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *imTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imTeamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMTeam(ctx, state.numID()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting IM team failed", err.Error())
	}
}

func (r *imTeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if n, err := strconv.ParseInt(strings.TrimSpace(req.ID), 10, 64); err != nil || n < 1 {
		resp.Diagnostics.AddError("Invalid import id", "Expected the numeric team id, got "+strconv.Quote(req.ID))
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
