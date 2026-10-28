package provider

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &statusPageMaintenanceResource{}
	_ resource.ResourceWithImportState = &statusPageMaintenanceResource{}
)

type statusPageMaintenanceResource struct{ client *client.Client }

type statusPageMaintenanceModel struct {
	ID                    types.String `tfsdk:"id"`
	PublicID              types.String `tfsdk:"public_id"`
	Title                 types.String `tfsdk:"title"`
	Description           types.String `tfsdk:"description"`
	ScheduledFor          types.String `tfsdk:"scheduled_for"`
	ScheduledUntil        types.String `tfsdk:"scheduled_until"`
	StatusPageIDs         types.Set    `tfsdk:"status_page_ids"`
	ComponentIDs          types.Set    `tfsdk:"component_ids"`
	NotifyOnSchedule      types.Bool   `tfsdk:"notify_on_schedule"`
	ReminderMinutesBefore types.Int64  `tfsdk:"reminder_minutes_before"`
	NotifyOnStart         types.Bool   `tfsdk:"notify_on_start"`
	NotifyOnComplete      types.Bool   `tfsdk:"notify_on_complete"`
	State                 types.String `tfsdk:"state"`
	CancelledAt           types.String `tfsdk:"cancelled_at"`
}

// rfc3339Offset is what the API accepts: a date-time with seconds and an explicit offset.
var rfc3339Offset = stringvalidator.RegexMatches(
	regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`),
	"must be RFC 3339 with an offset, e.g. 2026-11-03T22:00:00+01:00",
)

// keepInstant keeps the configured spelling of a timestamp while the API answers the same
// instant (it always answers UTC with milliseconds), so "22:00:00+01:00" does not drift.
func keepInstant(prior types.String, api string) types.String {
	if !prior.IsNull() && !prior.IsUnknown() && sameInstant(prior.ValueString(), api) {
		return prior
	}
	return types.StringValue(api)
}

// sameInstantPlan plans the state's spelling when the configured timestamp is the same instant,
// so an imported maintenance (state in UTC) shows no diff against a config with an offset.
type sameInstantPlan struct{}

func (sameInstantPlan) Description(context.Context) string {
	return "keeps the stored spelling while the instant is unchanged"
}
func (m sameInstantPlan) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }
func (sameInstantPlan) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}
	if sameInstant(req.PlanValue.ValueString(), req.StateValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

func sameInstant(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}

func (m *statusPageMaintenanceModel) fromAPI(ctx context.Context, x *client.StatusPageMaintenance) diag.Diagnostics {
	var diags, d diag.Diagnostics
	m.ID = types.StringValue(strconv.FormatInt(x.ID, 10))
	m.PublicID = types.StringValue(x.PublicID)
	m.Title = types.StringValue(x.Title)
	m.Description = types.StringPointerValue(x.Description)
	m.ScheduledFor = keepInstant(m.ScheduledFor, x.ScheduledFor)
	m.ScheduledUntil = keepInstant(m.ScheduledUntil, x.ScheduledUntil)
	m.StatusPageIDs, d = types.SetValueFrom(ctx, types.Int64Type, x.StatusPageIDs)
	diags.Append(d...)
	// Page-wide answers as []; keep an unset attribute unset instead of planning [] against null.
	if len(x.ComponentIDs) == 0 && (m.ComponentIDs.IsNull() || m.ComponentIDs.IsUnknown()) {
		m.ComponentIDs = types.SetNull(types.Int64Type)
	} else {
		m.ComponentIDs, d = types.SetValueFrom(ctx, types.Int64Type, x.ComponentIDs)
		diags.Append(d...)
	}
	m.NotifyOnSchedule = types.BoolValue(x.NotifyOnSchedule)
	m.ReminderMinutesBefore = types.Int64PointerValue(x.ReminderMinutesBefore)
	m.NotifyOnStart = types.BoolValue(x.NotifyOnStart)
	m.NotifyOnComplete = types.BoolValue(x.NotifyOnComplete)
	m.State = types.StringValue(x.State)
	m.CancelledAt = types.StringPointerValue(x.CancelledAt)
	return diags
}

func int64sOf(ctx context.Context, s types.Set) ([]int64, diag.Diagnostics) {
	out := []int64{}
	if s.IsNull() || s.IsUnknown() {
		return out, nil
	}
	diags := s.ElementsAs(ctx, &out, false)
	return out, diags
}

func NewStatusPageMaintenanceResource() resource.Resource { return &statusPageMaintenanceResource{} }

func (r *statusPageMaintenanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_maintenance"
}

func (r *statusPageMaintenanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A planned maintenance announced on one or more status pages, optionally for some of their components. " +
			"It starts and ends by itself at `scheduled_for`/`scheduled_until` and notifies subscribers as configured; " +
			"creating it announces it right away when `notify_on_schedule` is true. It does not silence monitor alerts " +
			"(that is `uptimeify_maintenance_window`). Ending a running maintenance early and cancelling an upcoming one are " +
			"lifecycle actions outside this resource (dashboard, API or MCP); a cancelled maintenance refuses further changes, " +
			"so change the configuration only after removing or replacing it. Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Numeric maintenance id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"title": schema.StringAttribute{Required: true, Description: "Public headline, 1 to 200 characters.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 200), statusPageTrimmed}},
			"description": schema.StringAttribute{Optional: true, Description: "What happens and what visitors notice, up to 5000 characters.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 5000), statusPageTrimmed}},
			"scheduled_for": schema.StringAttribute{Required: true,
				Description: "Start, RFC 3339 with offset. The configured spelling is kept while the instant is unchanged.",
				Validators:  []validator.String{rfc3339Offset}, PlanModifiers: []planmodifier.String{sameInstantPlan{}}},
			"scheduled_until": schema.StringAttribute{Required: true, Description: "End, RFC 3339 with offset, after `scheduled_for`.",
				Validators: []validator.String{rfc3339Offset}, PlanModifiers: []planmodifier.String{sameInstantPlan{}}},
			"status_page_ids": schema.SetAttribute{Required: true, ElementType: types.Int64Type,
				Description: "Numeric ids of the status pages that show the maintenance, 1 to 10.",
				Validators:  []validator.Set{setvalidator.SizeBetween(1, 10), setvalidator.ValueInt64sAre(int64validator.AtLeast(1))}},
			"component_ids": schema.SetAttribute{Optional: true, ElementType: types.Int64Type,
				Description: "Numeric ids of affected components (`component_id` of `uptimeify_status_page_component`), each on one of " +
					"the pages. Unset: the whole page.",
				Validators: []validator.Set{setvalidator.SizeBetween(1, 100), setvalidator.ValueInt64sAre(int64validator.AtLeast(1))}},
			"notify_on_schedule": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Announce to subscribers when saved and again when the start moves. Default `true`."},
			"reminder_minutes_before": schema.Int64Attribute{Optional: true,
				Description: "Remind subscribers this many minutes before the start, 5 to 10080. Unset: no reminder.",
				Validators:  []validator.Int64{int64validator.Between(5, 10080)}},
			"notify_on_start": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Notify subscribers when it begins. Default `false`."},
			"notify_on_complete": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Notify subscribers when it ends. Default `true`."},
			"state": schema.StringAttribute{Computed: true,
				Description: "`scheduled`, `in_progress`, `completed` or `cancelled`, derived from the clock at the last read."},
			"cancelled_at": schema.StringAttribute{Computed: true, Description: "When it was cancelled, if it was."},
		},
	}
}

func (r *statusPageMaintenanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageMaintenanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageMaintenanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pages, d := int64sOf(ctx, plan.StatusPageIDs)
	resp.Diagnostics.Append(d...)
	components, d := int64sOf(ctx, plan.ComponentIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{
		"title":            plan.Title.ValueString(),
		"scheduledFor":     plan.ScheduledFor.ValueString(),
		"scheduledUntil":   plan.ScheduledUntil.ValueString(),
		"statusPageIds":    pages,
		"componentIds":     components,
		"notifyOnSchedule": plan.NotifyOnSchedule.ValueBool(),
		"notifyOnStart":    plan.NotifyOnStart.ValueBool(),
		"notifyOnComplete": plan.NotifyOnComplete.ValueBool(),
	}
	if !plan.Description.IsNull() {
		body["description"] = plan.Description.ValueString()
	}
	if !plan.ReminderMinutesBefore.IsNull() {
		body["reminderMinutesBefore"] = plan.ReminderMinutesBefore.ValueInt64()
	}
	x, err := r.client.CreateStatusPageMaintenance(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating status page maintenance failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, x)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageMaintenanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageMaintenanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	x, err := r.client.GetStatusPageMaintenance(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page maintenance failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, x)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageMaintenanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusPageMaintenanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Title.Equal(state.Title) {
		patch["title"] = plan.Title.ValueString()
	}
	if !plan.Description.Equal(state.Description) {
		patch["description"] = plan.Description.ValueStringPointer() // nil clears it
	}
	if !sameInstant(plan.ScheduledFor.ValueString(), state.ScheduledFor.ValueString()) {
		patch["scheduledFor"] = plan.ScheduledFor.ValueString()
	}
	if !sameInstant(plan.ScheduledUntil.ValueString(), state.ScheduledUntil.ValueString()) {
		patch["scheduledUntil"] = plan.ScheduledUntil.ValueString()
	}
	// The API replaces pages and components together (pages alone make it page-wide), so both go.
	if !plan.StatusPageIDs.Equal(state.StatusPageIDs) || !plan.ComponentIDs.Equal(state.ComponentIDs) {
		pages, d := int64sOf(ctx, plan.StatusPageIDs)
		resp.Diagnostics.Append(d...)
		components, d := int64sOf(ctx, plan.ComponentIDs)
		resp.Diagnostics.Append(d...)
		patch["statusPageIds"] = pages
		patch["componentIds"] = components
	}
	if !plan.NotifyOnSchedule.Equal(state.NotifyOnSchedule) {
		patch["notifyOnSchedule"] = plan.NotifyOnSchedule.ValueBool()
	}
	if !plan.ReminderMinutesBefore.Equal(state.ReminderMinutesBefore) {
		patch["reminderMinutesBefore"] = plan.ReminderMinutesBefore.ValueInt64Pointer() // nil = no reminder
	}
	if !plan.NotifyOnStart.Equal(state.NotifyOnStart) {
		patch["notifyOnStart"] = plan.NotifyOnStart.ValueBool()
	}
	if !plan.NotifyOnComplete.Equal(state.NotifyOnComplete) {
		patch["notifyOnComplete"] = plan.NotifyOnComplete.ValueBool()
	}
	if resp.Diagnostics.HasError() {
		return
	}
	var x *client.StatusPageMaintenance
	var err error
	if len(patch) == 0 { // the API refuses an empty PATCH
		x, err = r.client.GetStatusPageMaintenance(ctx, state.ID.ValueString())
	} else {
		x, err = r.client.UpdateStatusPageMaintenance(ctx, state.ID.ValueString(), patch)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating status page maintenance failed", err.Error())
		return
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(plan.fromAPI(ctx, x)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageMaintenanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageMaintenanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteStatusPageMaintenance(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting status page maintenance failed", err.Error())
	}
}

func (r *statusPageMaintenanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
