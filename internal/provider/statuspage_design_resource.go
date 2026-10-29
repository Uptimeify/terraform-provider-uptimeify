package provider

import (
	"context"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &statusPageDesignResource{}
	_ resource.ResourceWithImportState = &statusPageDesignResource{}
)

type statusPageDesignResource struct{ client *client.Client }

type statusPageDesignModel struct {
	ID               types.String `tfsdk:"id"`
	StatusPageID     types.Int64  `tfsdk:"status_page_id"`
	Layout           types.String `tfsdk:"layout"`
	ColorScheme      types.String `tfsdk:"color_scheme"`
	AccentColor      types.String `tfsdk:"accent_color"`
	HeaderStyle      types.String `tfsdk:"header_style"`
	FontFamily       types.String `tfsdk:"font_family"`
	CardRadius       types.String `tfsdk:"card_radius"`
	PageWidth        types.String `tfsdk:"page_width"`
	CustomTitle      types.String `tfsdk:"custom_title"`
	SEOTitle         types.String `tfsdk:"seo_title"`
	CustomSubtitle   types.String `tfsdk:"custom_subtitle"`
	ShowPoweredBy    types.Bool   `tfsdk:"show_powered_by"`
	ShowUptimeStats  types.Bool   `tfsdk:"show_uptime_stats"`
	ShowServiceURLs  types.Bool   `tfsdk:"show_service_urls"`
	ShowLastChecked  types.Bool   `tfsdk:"show_last_checked"`
	ShowHistory      types.Bool   `tfsdk:"show_history"`
	HistoryDays      types.Int64  `tfsdk:"history_days"`
	ShowIncidents    types.Bool   `tfsdk:"show_incidents"`
	IncidentLimit    types.Int64  `tfsdk:"incident_limit"`
	IncidentDays     types.Int64  `tfsdk:"incident_days"`
	ShowMaintenance  types.Bool   `tfsdk:"show_maintenance"`
	MaintenanceLimit types.Int64  `tfsdk:"maintenance_limit"`
	MaintenanceDays  types.Int64  `tfsdk:"maintenance_days"`
	CustomCSS        types.String `tfsdk:"custom_css"`
	CustomCSSStored  types.String `tfsdk:"custom_css_stored"`
	StatusLabels     types.Map    `tfsdk:"status_labels"`
}

// statusLabelKeys: attribute map key -> API key in statusLabels.
var statusLabelKeys = map[string]string{
	"operational": "operational", "degraded": "degraded", "partial_outage": "partialOutage",
	"major_outage": "majorOutage", "maintenance": "maintenance",
}

// statusLabelsPatch: the map is the complete set of own texts, so every state it leaves out is
// reset (null) and the state equals the configuration after the merge-patch.
func statusLabelsPatch(ctx context.Context, m types.Map) map[string]any {
	in := map[string]string{}
	m.ElementsAs(ctx, &in, false)
	out := map[string]any{}
	for attrKey, apiKey := range statusLabelKeys {
		if v, ok := in[attrKey]; ok {
			out[apiKey] = v
		} else {
			out[apiKey] = nil
		}
	}
	return out
}

func statusLabelsFromAPI(labels map[string]string) types.Map {
	elems := map[string]attr.Value{}
	for attrKey, apiKey := range statusLabelKeys {
		if v, ok := labels[apiKey]; ok && v != "" {
			elems[attrKey] = types.StringValue(v)
		}
	}
	return types.MapValueMust(types.StringType, elems)
}

// designField ties one managed attribute to its API key; seo_title is read-only (not settable).
type designField struct {
	key string
	get func(*statusPageDesignModel) attr.Value
}

var designFields = []designField{
	{"layout", func(m *statusPageDesignModel) attr.Value { return m.Layout }},
	{"colorScheme", func(m *statusPageDesignModel) attr.Value { return m.ColorScheme }},
	{"accentColor", func(m *statusPageDesignModel) attr.Value { return m.AccentColor }},
	{"headerStyle", func(m *statusPageDesignModel) attr.Value { return m.HeaderStyle }},
	{"fontFamily", func(m *statusPageDesignModel) attr.Value { return m.FontFamily }},
	{"cardRadius", func(m *statusPageDesignModel) attr.Value { return m.CardRadius }},
	{"pageWidth", func(m *statusPageDesignModel) attr.Value { return m.PageWidth }},
	{"customTitle", func(m *statusPageDesignModel) attr.Value { return m.CustomTitle }},
	{"customSubtitle", func(m *statusPageDesignModel) attr.Value { return m.CustomSubtitle }},
	{"showPoweredBy", func(m *statusPageDesignModel) attr.Value { return m.ShowPoweredBy }},
	{"showUptimeStats", func(m *statusPageDesignModel) attr.Value { return m.ShowUptimeStats }},
	{"showServiceUrls", func(m *statusPageDesignModel) attr.Value { return m.ShowServiceURLs }},
	{"showLastChecked", func(m *statusPageDesignModel) attr.Value { return m.ShowLastChecked }},
	{"showHistory", func(m *statusPageDesignModel) attr.Value { return m.ShowHistory }},
	{"historyDays", func(m *statusPageDesignModel) attr.Value { return m.HistoryDays }},
	{"showIncidents", func(m *statusPageDesignModel) attr.Value { return m.ShowIncidents }},
	{"incidentLimit", func(m *statusPageDesignModel) attr.Value { return m.IncidentLimit }},
	{"incidentDays", func(m *statusPageDesignModel) attr.Value { return m.IncidentDays }},
	{"showMaintenance", func(m *statusPageDesignModel) attr.Value { return m.ShowMaintenance }},
	{"maintenanceLimit", func(m *statusPageDesignModel) attr.Value { return m.MaintenanceLimit }},
	{"maintenanceDays", func(m *statusPageDesignModel) attr.Value { return m.MaintenanceDays }},
	{"customCss", func(m *statusPageDesignModel) attr.Value { return m.CustomCSS }},
}

// designPatch holds every known, non-null plan value that differs from state (state nil = create).
func designPatch(plan, state *statusPageDesignModel) map[string]any {
	patch := map[string]any{}
	for _, f := range designFields {
		v := f.get(plan)
		if v.IsUnknown() || v.IsNull() || (state != nil && v.Equal(f.get(state))) {
			continue
		}
		switch v := v.(type) {
		case types.String:
			patch[f.key] = v.ValueString()
		case types.Bool:
			patch[f.key] = v.ValueBool()
		case types.Int64:
			patch[f.key] = v.ValueInt64()
		}
	}
	return patch
}

// withStatusLabels adds the status labels to patch when the plan sets them and they changed.
func withStatusLabels(ctx context.Context, patch map[string]any, plan, state *statusPageDesignModel) map[string]any {
	v := plan.StatusLabels
	if v.IsUnknown() || v.IsNull() || (state != nil && v.Equal(state.StatusLabels)) {
		return patch
	}
	patch["statusLabels"] = statusLabelsPatch(ctx, v)
	return patch
}

func (m *statusPageDesignModel) fromAPI(d *client.StatusPageDesign) {
	m.ID = types.StringValue(strconv.FormatInt(m.StatusPageID.ValueInt64(), 10))
	m.Layout = types.StringValue(d.Layout)
	m.ColorScheme = types.StringValue(d.ColorScheme)
	m.AccentColor = types.StringValue(d.AccentColor)
	m.HeaderStyle = types.StringValue(d.HeaderStyle)
	m.FontFamily = types.StringValue(d.FontFamily)
	m.CardRadius = types.StringValue(d.CardRadius)
	m.PageWidth = types.StringValue(d.PageWidth)
	m.CustomTitle = types.StringValue(d.CustomTitle)
	m.SEOTitle = types.StringValue(d.SEOTitle)
	m.CustomSubtitle = types.StringValue(d.CustomSubtitle)
	m.ShowPoweredBy = types.BoolValue(d.ShowPoweredBy)
	m.ShowUptimeStats = types.BoolValue(d.ShowUptimeStats)
	m.ShowServiceURLs = types.BoolValue(d.ShowServiceURLs)
	m.ShowLastChecked = types.BoolValue(d.ShowLastChecked)
	m.ShowHistory = types.BoolValue(d.ShowHistory)
	m.HistoryDays = types.Int64Value(d.HistoryDays)
	m.ShowIncidents = types.BoolValue(d.ShowIncidents)
	m.IncidentLimit = types.Int64Value(d.IncidentLimit)
	m.IncidentDays = types.Int64Value(d.IncidentDays)
	m.ShowMaintenance = types.BoolValue(d.ShowMaintenance)
	m.MaintenanceLimit = types.Int64Value(d.MaintenanceLimit)
	m.MaintenanceDays = types.Int64Value(d.MaintenanceDays)
	m.CustomCSS = types.StringValue(d.CustomCSS)
	m.CustomCSSStored = types.StringValue(d.CustomCSS)
	m.StatusLabels = statusLabelsFromAPI(d.StatusLabels)
}

// keepCSS: the server stores custom_css regenerated (minified), not as written. The written text
// stays in state while the stored form is the one this provider last saw; any other stored form
// is drift and replaces it.
func (m *statusPageDesignModel) keepCSS(written, lastStored types.String) {
	if !written.IsNull() && !written.IsUnknown() && (lastStored.IsUnknown() || lastStored.Equal(m.CustomCSSStored)) {
		m.CustomCSS = written
	}
}

func NewStatusPageDesignResource() resource.Resource { return &statusPageDesignResource{} }

func (r *statusPageDesignResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_design"
}

func (r *statusPageDesignResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	enum := func(desc string, vals ...string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: desc,
			Validators: []validator.String{stringvalidator.OneOf(vals...)}}
	}
	text := func(desc string, max int) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: desc,
			Validators: []validator.String{stringvalidator.LengthAtMost(max)}}
	}
	flag := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB, Description: desc}
	}
	num := func(desc string, lo, hi int64) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI, Description: desc,
			Validators: []validator.Int64{int64validator.Between(lo, hi)}}
	}
	resp.Schema = schema.Schema{
		Description: "The design of one status page (one per page). Unset arguments keep their current value; removing an " +
			"argument from the configuration does not reset it. Destroying the resource resets the whole design to the " +
			"defaults. Every option with examples: [Status page design](https://docs.uptimeify.io/status-pages/design). " +
			"Import with the numeric status page id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Same as `status_page_id`."},
			"status_page_id": schema.Int64Attribute{Required: true, Description: "Numeric status page id. Changing it replaces the resource.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:    []validator.Int64{int64validator.AtLeast(1)}},
			"layout": enum("Layout preset, with its name and caption in the design editor: `classic` (Classic, card and list), "+
				"`cards` (Cards, service grid), `minimal` (Minimal, clean lines), `sleek` (Sleek, dark with glow), "+
				"`board` (Board, color rows), `split` (Split, two columns), `timeline` (Timeline, event feed), "+
				"`compact` (Compact, dense table) or `statusboard` (Statusboard, uptime bars; the default). All show the "+
				"same data. Descriptions and screenshots: [Status page design](https://docs.uptimeify.io/status-pages/design#layouts).",
				"classic", "cards", "minimal", "sleek", "board", "split", "timeline", "compact", "statusboard"),
			"color_scheme": enum("`light`, `dark` or `auto` (default).", "light", "dark", "auto"),
			"accent_color": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "6-digit hex color, e.g. `#6366f1` (default).",
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^#[0-9a-fA-F]{6}$`), "must be a 6-digit hex color like #6366f1")}},
			"header_style":      enum("`simple` (default), `centered` or `hero`.", "simple", "centered", "hero"),
			"font_family":       enum("`system` (default) or `mono`.", "system", "mono"),
			"card_radius":       enum("`none`, `md` (default) or `xl`.", "none", "md", "xl"),
			"page_width":        enum("`sm`, `md`, `lg` (default) or `xl`.", "sm", "md", "lg", "xl"),
			"custom_title":      text("Visible page title, up to 120 characters. Empty = the page name.", 120),
			"seo_title":         schema.StringAttribute{Computed: true, Description: "`<title>` override; not settable through the API."},
			"custom_subtitle":   text("Up to 200 characters.", 200),
			"show_powered_by":   flag("Default `true`."),
			"show_uptime_stats": flag("Default `true`."),
			"show_service_urls": flag("Show monitor URLs. Default `false`."),
			"show_last_checked": flag("Default `false`."),
			"show_history":      flag("Show availability bars. Default `true`."),
			"history_days":      num("Window of the availability bars, 1 to 3650 days. Default `30`.", 1, 3650),
			"show_incidents":    flag("Default `true`. Same switch as `show_recent_incidents` of the page API."),
			"incident_limit":    num("1 to 50 incidents. Default `5`.", 1, 50),
			"incident_days":     num("1 to 3650 days. Default `30`.", 1, 3650),
			"show_maintenance":  flag("Default `true`."),
			"maintenance_limit": num("1 to 50 maintenance windows. Default `5`.", 1, 50),
			"maintenance_days":  num("How far back past maintenance is shown, 1 to 3650 days. Default `30`.", 1, 3650),
			"custom_css": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS,
				Description: "Own CSS for the public page only, at most 20000 bytes; empty string = none (default). No `url()`, " +
					"`@import`, `@font-face` or other resource loads; a rejected stylesheet fails the apply. Kept as written " +
					"in state; the server's regenerated form is `custom_css_stored`.",
				Validators: []validator.String{stringvalidator.LengthAtMost(20000)}},
			"status_labels": schema.MapAttribute{ElementType: types.StringType, Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
				Description: "Own public texts per state, used by every layout, the badge and the widget. Keys: `operational`, " +
					"`degraded`, `partial_outage`, `major_outage`, `maintenance`; values 1 to 40 characters, no surrounding " +
					"whitespace. When set, the map is the complete list: a state left out shows its default text.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.OneOf("operational", "degraded", "partial_outage", "major_outage", "maintenance")),
					mapvalidator.ValueStringsAre(stringvalidator.LengthBetween(1, 40), statusPageTrimmed),
				}},
			"custom_css_stored": schema.StringAttribute{Computed: true,
				Description: "`custom_css` as the server stored it (regenerated and minified). A change made outside Terraform shows up here and in `custom_css`."},
		},
	}
}

func (r *statusPageDesignResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *statusPageDesignResource) write(ctx context.Context, plan *statusPageDesignModel, patch map[string]any) (*client.StatusPageDesign, error) {
	page := strconv.FormatInt(plan.StatusPageID.ValueInt64(), 10)
	if len(patch) == 0 { // the API refuses an empty PATCH
		return r.client.GetStatusPageDesign(ctx, page)
	}
	return r.client.UpdateStatusPageDesign(ctx, page, patch)
}

func (r *statusPageDesignResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageDesignModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.write(ctx, &plan, withStatusLabels(ctx, designPatch(&plan, nil), &plan, nil))
	if err != nil {
		resp.Diagnostics.AddError("Setting status page design failed", err.Error())
		return
	}
	written := plan.CustomCSS
	plan.fromAPI(d)
	plan.keepCSS(written, types.StringUnknown())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageDesignResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageDesignModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetStatusPageDesign(ctx, strconv.FormatInt(state.StatusPageID.ValueInt64(), 10))
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading status page design failed", err.Error())
		return
	}
	written, lastStored := state.CustomCSS, state.CustomCSSStored
	state.fromAPI(d)
	state.keepCSS(written, lastStored)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageDesignResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusPageDesignModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.write(ctx, &plan, withStatusLabels(ctx, designPatch(&plan, &state), &plan, &state))
	if err != nil {
		resp.Diagnostics.AddError("Updating status page design failed", err.Error())
		return
	}
	written := plan.CustomCSS
	plan.fromAPI(d)
	plan.keepCSS(written, types.StringUnknown())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete resets the design to the defaults (designConfig null); the page itself stays.
func (r *statusPageDesignResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageDesignModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.ResetStatusPageDesign(ctx, strconv.FormatInt(state.StatusPageID.ValueInt64(), 10))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Resetting status page design failed", err.Error())
	}
}

func (r *statusPageDesignResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import id", "expected the numeric status page id, got "+strconv.Quote(req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
