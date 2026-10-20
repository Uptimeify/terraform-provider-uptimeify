package provider

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.ResourceWithConfigure        = &maintenanceWindowResource{}
	_ resource.ResourceWithImportState      = &maintenanceWindowResource{}
	_ resource.ResourceWithConfigValidators = &maintenanceWindowResource{}
)

type maintenanceWindowResource struct{ client *client.Client }

type maintenanceTargetModel struct {
	Type types.String `tfsdk:"type"`
	ID   types.Int64  `tfsdk:"id"`
}

type recurrenceModel struct {
	Frequency         types.String `tfsdk:"frequency"`
	IntervalCount     types.Int64  `tfsdk:"interval_count"`
	DaysOfWeek        []int64      `tfsdk:"days_of_week"`
	DayOfMonth        types.Int64  `tfsdk:"day_of_month"`
	EndRecurrenceDate types.String `tfsdk:"end_recurrence_date"`
}

// maintenanceWindowModel is shared by the uptimeify_maintenance_window resource and data source.
type maintenanceWindowModel struct {
	ID                types.String             `tfsdk:"id"`
	PublicID          types.String             `tfsdk:"public_id"`
	Name              types.String             `tfsdk:"name"`
	Description       types.String             `tfsdk:"description"`
	StartTime         types.String             `tfsdk:"start_time"`
	EndTime           types.String             `tfsdk:"end_time"`
	Timezone          types.String             `tfsdk:"timezone"`
	IsActive          types.Bool               `tfsdk:"is_active"`
	CustomerID        types.Int64              `tfsdk:"customer_id"`
	Targets           []maintenanceTargetModel `tfsdk:"targets"`
	TagIDs            []int64                  `tfsdk:"tag_ids"`
	RecurrencePattern *recurrenceModel         `tfsdk:"recurrence_pattern"`
	CreatedAt         types.String             `tfsdk:"created_at"`
	UpdatedAt         types.String             `tfsdk:"updated_at"`
}

// fromAPI fills m from the GET answer. prior is the plan or state the call started from: a
// start/end time written in another RFC3339 spelling of the same instant is kept as written.
func (m *maintenanceWindowModel) fromAPI(w *client.MaintenanceWindow, prior *maintenanceWindowModel) {
	m.ID = types.StringValue(strconv.FormatInt(w.ID, 10))
	m.PublicID = types.StringValue(w.PublicID)
	m.Name = types.StringValue(w.Name)
	m.Description = types.StringPointerValue(w.Description)
	m.StartTime = keepSameInstant(prior.StartTime, w.StartTime)
	m.EndTime = keepSameInstant(prior.EndTime, w.EndTime)
	m.Timezone = types.StringValue(w.Timezone)
	m.IsActive = types.BoolValue(w.IsActive)
	m.CreatedAt = types.StringValue(w.CreatedAt)
	m.UpdatedAt = types.StringValue(w.UpdatedAt)

	// customerId is derived from the targets for targeted windows; only a window without
	// targets and tags is a whole-customer window, and only then is customer_id configuration.
	m.CustomerID = types.Int64Null()
	if len(w.Targets) == 0 && len(w.Tags) == 0 {
		m.CustomerID = types.Int64PointerValue(w.CustomerID)
	}
	m.Targets = nil
	for _, t := range w.Targets {
		m.Targets = append(m.Targets, maintenanceTargetModel{Type: types.StringValue(t.Type), ID: types.Int64Value(t.ID)})
	}
	m.TagIDs = nil
	for _, t := range w.Tags {
		m.TagIDs = append(m.TagIDs, t.ID)
	}

	m.RecurrencePattern = nil
	if p := w.RecurrencePattern; w.IsRecurring && p != nil {
		r := &recurrenceModel{
			Frequency:         types.StringValue(p.Frequency),
			IntervalCount:     types.Int64PointerValue(p.IntervalCount),
			DayOfMonth:        types.Int64PointerValue(p.DayOfMonth),
			EndRecurrenceDate: types.StringPointerValue(p.EndRecurrenceDate),
		}
		if len(p.DaysOfWeek) > 0 {
			r.DaysOfWeek = p.DaysOfWeek
		}
		m.RecurrencePattern = r
	}
}

func keepSameInstant(prior types.String, api string) types.String {
	if prior.IsNull() || prior.IsUnknown() {
		return types.StringValue(api)
	}
	a, errA := time.Parse(time.RFC3339, prior.ValueString())
	b, errB := time.Parse(time.RFC3339, api)
	if errA == nil && errB == nil && a.Equal(b) {
		return prior
	}
	return types.StringValue(api)
}

func (m *maintenanceWindowModel) apiTargets() []client.MaintenanceTarget {
	out := []client.MaintenanceTarget{}
	for _, t := range m.Targets {
		out = append(out, client.MaintenanceTarget{Type: t.Type.ValueString(), ID: t.ID.ValueInt64()})
	}
	return out
}

func (m *maintenanceWindowModel) apiTagIDs() []int64 {
	if m.TagIDs == nil {
		return []int64{}
	}
	return m.TagIDs
}

func (m *maintenanceWindowModel) apiRecurrence() *client.RecurrencePattern {
	r := m.RecurrencePattern
	if r == nil {
		return nil
	}
	return &client.RecurrencePattern{
		Frequency:         r.Frequency.ValueString(),
		IntervalCount:     r.IntervalCount.ValueInt64Pointer(),
		DaysOfWeek:        r.DaysOfWeek,
		DayOfMonth:        r.DayOfMonth.ValueInt64Pointer(),
		EndRecurrenceDate: r.EndRecurrenceDate.ValueStringPointer(),
	}
}

func NewMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }

func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

// rfc3339Validator accepts RFC3339 timestamps (with Z or an offset).
type rfc3339Validator struct{}

func (rfc3339Validator) Description(context.Context) string { return "must be an RFC3339 timestamp" }
func (v rfc3339Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (rfc3339Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.Parse(time.RFC3339, req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid timestamp",
			fmt.Sprintf("%q is not RFC3339, e.g. 2030-01-01T02:00:00Z.", req.ConfigValue.ValueString()))
	}
}

func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	timeDesc := " RFC3339 timestamp. The API answers in UTC with milliseconds (`2030-01-01T02:00:00.000Z`); " +
		"another spelling of the same instant is kept as written."
	resp.Schema = schema.Schema{
		Description: "A maintenance window: alerts and incidents of its targets are suppressed while it runs. " +
			"Set `targets` and/or `tag_ids`, or `customer_id` alone for the whole customer. " +
			"A window with only organization-wide tags applies organization-wide. Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric window id."},
			"public_id":   schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Public UUID of the window."},
			"name":        schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"description": schema.StringAttribute{Optional: true, Description: "At most 2000 characters.", Validators: []validator.String{stringvalidator.LengthBetween(1, 2000)}},
			"start_time":  schema.StringAttribute{Required: true, Description: "Start." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
			"end_time":    schema.StringAttribute{Required: true, Description: "End, after start_time." + timeDesc, Validators: []validator.String{rfc3339Validator{}}},
			"timezone": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("UTC"),
				Description: "IANA zone the recurrence is read in, e.g. `Europe/Berlin`. Use the canonical name: the API stores aliases (`utc`, `Japan`) under their canonical spelling. Default `UTC`.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 64)},
			},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Default `true`."},
			"customer_id": schema.Int64Attribute{
				Optional:      true,
				Description:   "Whole-customer window: every monitor of this customer. Cannot be combined with `targets` or `tag_ids`; changing it replaces the window.",
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"targets": schema.SetNestedAttribute{
				Optional:    true,
				Description: "Monitors the window applies to. All targets must belong to one customer.",
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:    true,
						Description: "One of `" + strings.Join(client.MaintenanceTargetTypes, "`, `") + "`.",
						Validators:  []validator.String{stringvalidator.OneOf(client.MaintenanceTargetTypes...)},
					},
					"id": schema.Int64Attribute{Required: true, Description: "Numeric id of the monitor.", Validators: []validator.Int64{int64validator.AtLeast(1)}},
				}},
			},
			"tag_ids": schema.SetAttribute{
				ElementType: types.Int64Type,
				Optional:    true,
				Description: "Numeric tag ids: every monitor carrying one of the tags is in maintenance.",
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"recurrence_pattern": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Repeat the window. Unset = one-off window.",
				Attributes: map[string]schema.Attribute{
					"frequency": schema.StringAttribute{Required: true, Description: "`daily`, `weekly` or `monthly`.",
						Validators: []validator.String{stringvalidator.OneOf("daily", "weekly", "monthly")}},
					"interval_count": schema.Int64Attribute{Optional: true, Description: "Repeat every N units of frequency (2 with weekly = every other week), 1 to 365. The API treats unset as 1.",
						Validators: []validator.Int64{int64validator.Between(1, 365)}},
					"days_of_week": schema.ListAttribute{ElementType: types.Int64Type, Optional: true, Description: "Weekdays, 0 = Sunday to 6 = Saturday.",
						Validators: []validator.List{listvalidator.SizeBetween(1, 7), listvalidator.ValueInt64sAre(int64validator.Between(0, 6))}},
					"day_of_month": schema.Int64Attribute{Optional: true, Description: "Day of month, 1 to 31.",
						Validators: []validator.Int64{int64validator.Between(1, 31)}},
					"end_recurrence_date": schema.StringAttribute{Optional: true, Description: "Last date of the recurrence, e.g. `2031-12-31` or an RFC3339 timestamp.",
						Validators: []validator.String{stringvalidator.LengthBetween(1, 64)}},
				},
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *maintenanceWindowResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.AtLeastOneOf(path.MatchRoot("customer_id"), path.MatchRoot("targets"), path.MatchRoot("tag_ids")),
		resourcevalidator.Conflicting(path.MatchRoot("customer_id"), path.MatchRoot("targets")),
		resourcevalidator.Conflicting(path.MatchRoot("customer_id"), path.MatchRoot("tag_ids")),
	}
}

func (r *maintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateMaintenanceWindow(ctx, client.MaintenanceWindowCreate{
		Name:              plan.Name.ValueString(),
		Description:       plan.Description.ValueStringPointer(),
		StartTime:         plan.StartTime.ValueString(),
		EndTime:           plan.EndTime.ValueString(),
		IsRecurring:       plan.RecurrencePattern != nil,
		RecurrencePattern: plan.apiRecurrence(),
		IsActive:          plan.IsActive.ValueBool(),
		Timezone:          plan.Timezone.ValueString(),
		CustomerID:        plan.CustomerID.ValueInt64Pointer(),
		Targets:           plan.apiTargets(),
		TagIDs:            plan.TagIDs,
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating maintenance window failed", err.Error())
		return
	}
	w, err := r.client.GetMaintenanceWindow(ctx, strconv.FormatInt(created.ID, 10))
	if err != nil {
		// Keep the created window in state (tainted) instead of orphaning it.
		resp.Diagnostics.AddError("Reading maintenance window after create failed", err.Error())
		w = created
	}
	prior := plan
	plan.fromAPI(w, &prior)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// readInto GETs the window (POST and PATCH answer without targets/tags) and fills m.
func (r *maintenanceWindowResource) readInto(ctx context.Context, id string, m *maintenanceWindowModel, addError func(string, string)) {
	w, err := r.client.GetMaintenanceWindow(ctx, id)
	if err != nil {
		addError("Reading maintenance window failed", err.Error())
		return
	}
	prior := *m
	m.fromAPI(w, &prior)
}

func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetMaintenanceWindow(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading maintenance window failed", err.Error())
		return
	}
	prior := state
	state.fromAPI(w, &prior)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// maintenancePatch holds only the changed keys. Targets and tags travel together: the API
// re-derives the window's customer from both, so sending one alone could re-anchor the window.
func maintenancePatch(plan, state *maintenanceWindowModel) map[string]any {
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Description.Equal(state.Description) {
		patch["description"] = plan.Description.ValueStringPointer() // nil clears it
	}
	if !plan.StartTime.Equal(state.StartTime) {
		patch["startTime"] = plan.StartTime.ValueString()
	}
	if !plan.EndTime.Equal(state.EndTime) {
		patch["endTime"] = plan.EndTime.ValueString()
	}
	if !plan.Timezone.Equal(state.Timezone) {
		patch["timezone"] = plan.Timezone.ValueString()
	}
	if !plan.IsActive.Equal(state.IsActive) {
		patch["isActive"] = plan.IsActive.ValueBool()
	}
	if !reflect.DeepEqual(plan.RecurrencePattern, state.RecurrencePattern) {
		patch["isRecurring"] = plan.RecurrencePattern != nil
		patch["recurrencePattern"] = plan.apiRecurrence() // nil clears it
	}
	if !sameTargets(plan.Targets, state.Targets) || !sameInt64s(plan.TagIDs, state.TagIDs) {
		patch["targets"] = plan.apiTargets()
		patch["tagIds"] = plan.apiTagIDs()
	}
	return patch
}

func sameTargets(a, b []maintenanceTargetModel) bool {
	key := func(ts []maintenanceTargetModel) []string {
		out := make([]string, 0, len(ts))
		for _, t := range ts {
			out = append(out, t.Type.ValueString()+":"+strconv.FormatInt(t.ID.ValueInt64(), 10))
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(key(a), key(b))
}

func sameInt64s(a, b []int64) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func (r *maintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if patch := maintenancePatch(&plan, &state); len(patch) > 0 {
		if err := r.client.UpdateMaintenanceWindow(ctx, state.ID.ValueString(), patch); err != nil {
			resp.Diagnostics.AddError("Updating maintenance window failed", err.Error())
			return
		}
	}
	r.readInto(ctx, state.ID.ValueString(), &plan, resp.Diagnostics.AddError)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMaintenanceWindow(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting maintenance window failed", err.Error())
	}
}

func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
