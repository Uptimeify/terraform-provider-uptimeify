package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// ReportMonitorTypes is MONITOR_TYPES (server/lib/reports/monitor-types.ts).
var ReportMonitorTypes = []string{"website", "dns", "dnsbl", "domain", "icmp", "smtp", "ssh", "tcp", "ftp", "imap"}

var (
	_ resource.ResourceWithConfigure   = &reportResource{}
	_ resource.ResourceWithImportState = &reportResource{}
)

var (
	reportSignalsAttrTypes  = map[string]attr.Type{"incident": types.BoolType, "downtime_minutes": types.Int64Type, "ssl_days_lt": types.Int64Type, "response_breach": types.BoolType}
	reportSectionsAttrTypes = map[string]attr.Type{"fleet_summary": types.BoolType, "worst_performers": types.BoolType, "per_site_table": types.BoolType, "incident_log": types.BoolType, "ssl_expiry": types.BoolType, "group_by_customer": types.BoolType}
	reportSectionsByType    = types.MapType{ElemType: types.BoolType}
)

type reportResource struct{ client *client.Client }

type reportModel struct {
	ID                  types.String  `tfsdk:"id"`
	Name                types.String  `tfsdk:"name"`
	Enabled             types.Bool    `tfsdk:"enabled"`
	Frequency           types.String  `tfsdk:"frequency"`
	Weekday             types.Int64   `tfsdk:"weekday"`
	DayOfMonth          types.Int64   `tfsdk:"day_of_month"`
	SendHour            types.Int64   `tfsdk:"send_hour"`
	Timezone            types.String  `tfsdk:"timezone"`
	ScopeMode           types.String  `tfsdk:"scope_mode"`
	ScopeCustomerIDs    types.List    `tfsdk:"scope_customer_ids"`
	ScopeTagIDs         types.List    `tfsdk:"scope_tag_ids"`
	InclusionMode       types.String  `tfsdk:"inclusion_mode"`
	ProblemSignals      types.Object  `tfsdk:"problem_signals"`
	ThresholdUptimeLt   types.Float64 `tfsdk:"threshold_uptime_lt"`
	ThresholdResponseGt types.Int64   `tfsdk:"threshold_response_gt"`
	SendWhenEmpty       types.Bool    `tfsdk:"send_when_empty"`
	Sections            types.Object  `tfsdk:"sections"`
	MonitorTypes        types.List    `tfsdk:"monitor_types"`
	SectionsByType      types.Map     `tfsdk:"sections_by_type"`
	GroupByCustomer     types.Bool    `tfsdk:"group_by_customer"`
	IncludeStatusPages  types.Bool    `tfsdk:"include_status_pages"`
	Format              types.String  `tfsdk:"format"`
	RecipientEmails     types.List    `tfsdk:"recipient_emails"`
	RecipientUserIDs    types.List    `tfsdk:"recipient_user_ids"`
	CreatedAt           types.String  `tfsdk:"created_at"`
}

func (m *reportModel) fromAPI(ctx context.Context, r *client.Report) diag.Diagnostics {
	var diags, d diag.Diagnostics
	add := func(v diag.Diagnostics) { diags.Append(v...) }
	m.ID = types.StringValue(r.ID)
	m.Name = types.StringValue(r.Name)
	m.Enabled = types.BoolValue(r.Enabled)
	m.Frequency = types.StringValue(r.Frequency)
	m.Weekday = types.Int64PointerValue(r.Weekday)
	m.DayOfMonth = types.Int64PointerValue(r.DayOfMonth)
	m.SendHour = types.Int64Value(r.SendHour)
	m.Timezone = types.StringValue(r.Timezone)
	m.ScopeMode = types.StringValue(r.ScopeMode)
	m.ScopeCustomerIDs, d = types.ListValueFrom(ctx, types.Int64Type, nonNil(r.ScopeCustomerIDs))
	add(d)
	m.ScopeTagIDs, d = types.ListValueFrom(ctx, types.Int64Type, nonNil(r.ScopeTagIDs))
	add(d)
	m.InclusionMode = types.StringValue(r.InclusionMode)
	m.ProblemSignals, d = types.ObjectValueFrom(ctx, reportSignalsAttrTypes, r.ProblemSignals)
	add(d)
	m.ThresholdUptimeLt = types.Float64PointerValue(r.ThresholdUptimeLt)
	m.ThresholdResponseGt = types.Int64PointerValue(r.ThresholdResponseGt)
	m.SendWhenEmpty = types.BoolValue(r.SendWhenEmpty)
	m.Sections, d = types.ObjectValueFrom(ctx, reportSectionsAttrTypes, r.Sections)
	add(d)
	m.MonitorTypes, d = types.ListValueFrom(ctx, types.StringType, nonNil(r.MonitorTypes))
	add(d)
	sbt := r.SectionsByType
	if sbt == nil {
		sbt = map[string]map[string]bool{}
	}
	m.SectionsByType, d = types.MapValueFrom(ctx, reportSectionsByType, sbt)
	add(d)
	m.GroupByCustomer = types.BoolValue(r.GroupByCustomer)
	m.IncludeStatusPages = types.BoolValue(r.IncludeStatusPages)
	m.Format = types.StringValue(r.Format)
	m.RecipientEmails, d = types.ListValueFrom(ctx, types.StringType, nonNil(r.RecipientEmails))
	add(d)
	m.RecipientUserIDs, d = types.ListValueFrom(ctx, types.StringType, nonNil(r.RecipientUserIDs))
	add(d)
	m.CreatedAt = types.StringValue(r.CreatedAt)
	return diags
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// body is the API form of every known value; the four nullable attributes are always present
// (nil = null) so that removing one clears it.
func (m *reportModel) body(ctx context.Context) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	b := map[string]any{}
	known := func(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }
	str := func(k string, v types.String) {
		if known(v) {
			b[k] = v.ValueString()
		}
	}
	boolean := func(k string, v types.Bool) {
		if known(v) {
			b[k] = v.ValueBool()
		}
	}
	list := func(k string, v types.List, out any) {
		if known(v) {
			diags.Append(v.ElementsAs(ctx, out, false)...)
			b[k] = out
		}
	}
	str("name", m.Name)
	boolean("enabled", m.Enabled)
	str("frequency", m.Frequency)
	if !m.Weekday.IsUnknown() {
		b["weekday"] = m.Weekday.ValueInt64Pointer()
	}
	if !m.DayOfMonth.IsUnknown() {
		b["dayOfMonth"] = m.DayOfMonth.ValueInt64Pointer()
	}
	if known(m.SendHour) {
		b["sendHour"] = m.SendHour.ValueInt64()
	}
	str("timezone", m.Timezone)
	str("scopeMode", m.ScopeMode)
	list("scopeCustomerIds", m.ScopeCustomerIDs, &[]int64{})
	list("scopeTagIds", m.ScopeTagIDs, &[]int64{})
	str("inclusionMode", m.InclusionMode)
	if known(m.ProblemSignals) {
		var ps client.ProblemSignals
		diags.Append(m.ProblemSignals.As(ctx, &ps, basetypes.ObjectAsOptions{})...)
		b["problemSignals"] = ps
	}
	if !m.ThresholdUptimeLt.IsUnknown() {
		b["thresholdUptimeLt"] = m.ThresholdUptimeLt.ValueFloat64Pointer()
	}
	if !m.ThresholdResponseGt.IsUnknown() {
		b["thresholdResponseGt"] = m.ThresholdResponseGt.ValueInt64Pointer()
	}
	boolean("sendWhenEmpty", m.SendWhenEmpty)
	if known(m.Sections) {
		var s client.ReportSections
		diags.Append(m.Sections.As(ctx, &s, basetypes.ObjectAsOptions{})...)
		b["sections"] = s
	}
	list("monitorTypes", m.MonitorTypes, &[]string{})
	if known(m.SectionsByType) {
		sbt := map[string]map[string]bool{}
		diags.Append(m.SectionsByType.ElementsAs(ctx, &sbt, false)...)
		b["sectionsByType"] = sbt
	}
	boolean("groupByCustomer", m.GroupByCustomer)
	boolean("includeStatusPages", m.IncludeStatusPages)
	str("format", m.Format)
	list("recipientEmails", m.RecipientEmails, &[]string{})
	list("recipientUserIds", m.RecipientUserIDs, &[]string{})
	return b, diags
}

func NewReportResource() resource.Resource { return &reportResource{} }

func (r *reportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report"
}

func (r *reportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	keepL := []planmodifier.List{listplanmodifier.UseStateForUnknown()}
	keepO := []planmodifier.Object{objectplanmodifier.UseStateForUnknown()}
	optS := func(desc string, v ...validator.String) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: desc, Validators: v}
	}
	optB := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB, Description: desc}
	}
	req := func() schema.BoolAttribute { return schema.BoolAttribute{Required: true} }
	resp.Schema = schema.Schema{
		Description: "A scheduled fleet report of the organization (mail, optionally with PDF). Unset optional attributes " +
			"take the server default on create and keep their value afterwards. Import with the report id (UUID).",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Report id (UUID)."},
			"name":      schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 200)}},
			"enabled":   optB("Default `true`."),
			"frequency": optS("`daily`, `weekly` or `monthly` (default). Weekly needs `weekday`, monthly needs `day_of_month`.", stringvalidator.OneOf("daily", "weekly", "monthly")),
			"weekday": schema.Int64Attribute{Optional: true, Description: "0-6, 0 = Sunday. Required for weekly reports.",
				Validators: []validator.Int64{int64validator.Between(0, 6)}},
			"day_of_month": schema.Int64Attribute{Optional: true, Description: "1-28. Required for monthly reports.",
				Validators: []validator.Int64{int64validator.Between(1, 28)}},
			"send_hour": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Description: "Hour of day 0-23 in `timezone`. Default 2.", Validators: []validator.Int64{int64validator.Between(0, 23)}},
			"timezone":   optS("IANA time zone. Default `Europe/Berlin`.", stringvalidator.LengthBetween(1, 64)),
			"scope_mode": optS("`all` (default), `customers` or `tags`.", stringvalidator.OneOf("all", "customers", "tags")),
			"scope_customer_ids": schema.ListAttribute{ElementType: types.Int64Type, Optional: true, Computed: true, PlanModifiers: keepL,
				Description: "Customers for `scope_mode = \"customers\"`."},
			"scope_tag_ids": schema.ListAttribute{ElementType: types.Int64Type, Optional: true, Computed: true, PlanModifiers: keepL,
				Description: "Tags for `scope_mode = \"tags\"`."},
			"inclusion_mode": optS("`all` (default), `problems` or `threshold`.", stringvalidator.OneOf("all", "problems", "threshold")),
			"problem_signals": schema.SingleNestedAttribute{Optional: true, Computed: true, PlanModifiers: keepO,
				Description: "What counts as a problem for `inclusion_mode = \"problems\"`.",
				Attributes: map[string]schema.Attribute{
					"incident":         req(),
					"downtime_minutes": schema.Int64Attribute{Optional: true, Validators: []validator.Int64{int64validator.AtLeast(1)}},
					"ssl_days_lt":      schema.Int64Attribute{Optional: true, Validators: []validator.Int64{int64validator.AtLeast(1)}},
					"response_breach":  req(),
				}},
			"threshold_uptime_lt": schema.Float64Attribute{Optional: true, Description: "Uptime percentage for `inclusion_mode = \"threshold\"`.",
				Validators: []validator.Float64{float64validator.Between(0, 100)}},
			"threshold_response_gt": schema.Int64Attribute{Optional: true, Description: "Response time in ms for `inclusion_mode = \"threshold\"`.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"send_when_empty": optB("Send even when nothing qualifies. Default `false`."),
			"sections": schema.SingleNestedAttribute{Optional: true, Computed: true, PlanModifiers: keepO,
				Description: "Report sections (default: all on except `group_by_customer`).",
				Attributes: map[string]schema.Attribute{
					"fleet_summary": req(), "worst_performers": req(), "per_site_table": req(),
					"incident_log": req(), "ssl_expiry": req(), "group_by_customer": req(),
				}},
			"monitor_types": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, PlanModifiers: keepL,
				Description: "Monitor types in the report (default: all ten).",
				Validators: []validator.List{listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.OneOf(ReportMonitorTypes...))}},
			"sections_by_type": schema.MapAttribute{ElementType: reportSectionsByType, Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
				Description:   "Per monitor type section switches, e.g. `{ website = { sslExpiry = false } }` (server-side key names)."},
			"group_by_customer": optB("Group by customer. Default `false`."),
			"include_status_pages": optB("Append the status pages section (published state, customer, components, incidents, " +
				"maintenance, subscribers). Scope `customers` lists those customers' pages, scope `tags` the pages showing a " +
				"monitor with one of the tags. Default `true`."),
			"format": optS("`email` or `email_pdf` (default).", stringvalidator.OneOf("email", "email_pdf")),
			"recipient_emails": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, PlanModifiers: keepL,
				Validators: []validator.List{listvalidator.SizeAtMost(50)}},
			"recipient_user_ids": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, PlanModifiers: keepL,
				Description: "Logins of the organization (`uptimeify_user.id`).", Validators: []validator.List{listvalidator.SizeAtMost(50)}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
		},
	}
}

func (r *reportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *reportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, d := plan.body(ctx)
	if resp.Diagnostics.Append(d...); resp.Diagnostics.HasError() {
		return
	}
	rep, err := r.client.CreateReport(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating report failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, rep)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *reportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rep, err := r.client.GetReport(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading report failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, rep)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *reportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state reportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pb, d1 := plan.body(ctx)
	sb, d2 := state.body(ctx)
	resp.Diagnostics.Append(d1...)
	resp.Diagnostics.Append(d2...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	for k, v := range pb {
		a, _ := json.Marshal(v)
		b, _ := json.Marshal(sb[k])
		if string(a) != string(b) {
			patch[k] = v
		}
	}
	var rep *client.Report
	var err error
	if len(patch) == 0 { // an empty PATCH is a 400
		rep, err = r.client.GetReport(ctx, state.ID.ValueString())
	} else {
		rep, err = r.client.UpdateReport(ctx, state.ID.ValueString(), patch)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating report failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, rep)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *reportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state reportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteReport(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting report failed", err.Error())
	}
}

func (r *reportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
