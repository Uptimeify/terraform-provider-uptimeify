package provider

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// The seven protocol monitor types (dns, tcp, icmp, smtp, ssh, ftp, imap-pop) share one CRUD flow
// and the attributes in monitorBase; each type adds only its model fields, attributes and the
// mapping of its config (protocol_monitor_types.go).

// monitorBase holds the attributes every protocol monitor has (contract `base` + patchCommon).
type monitorBase struct {
	ID                       types.String `tfsdk:"id"`
	PublicID                 types.String `tfsdk:"public_id"`
	OrganizationID           types.Int64  `tfsdk:"organization_id"`
	CustomerID               types.Int64  `tfsdk:"customer_id"`
	Name                     types.String `tfsdk:"name"`
	Hostname                 types.String `tfsdk:"hostname"`
	CheckInterval            types.Int64  `tfsdk:"check_interval"`
	TimeoutSeconds           types.Int64  `tfsdk:"timeout_seconds"`
	Status                   types.String `tfsdk:"status"`
	ManagementType           types.String `tfsdk:"management_type"`
	AllowedCheckCountryCodes types.Set    `tfsdk:"allowed_check_country_codes"`
	CreatedAt                types.String `tfsdk:"created_at"`
	UpdatedAt                types.String `tfsdk:"updated_at"`
	AllowDuplicate           types.Bool   `tfsdk:"allow_duplicate"`
}

func (b *monitorBase) base() *monitorBase { return b }

// monitorModel is implemented by *<type>MonitorModel, which embeds monitorBase.
type monitorModel[M any] interface {
	*M
	base() *monitorBase
	// put adds the type-specific body keys. cfg is the configuration (the only place write-only
	// values are readable), prior the state; on create prior is the zero model and b.create is set.
	put(ctx context.Context, b *bodyBuilder, cfg, prior *M) diag.Diagnostics
	// read copies the type-specific fields from an API row.
	read(ctx context.Context, row *client.MonitorRow) diag.Diagnostics
}

// monitorSpec describes one monitor type.
type monitorSpec struct {
	name        string // type name suffix, e.g. "dns_monitor"
	kind        string // API path segment, e.g. "dns-monitors"
	title       string // e.g. "DNS monitor"
	description string
	attrs       map[string]schema.Attribute
}

// bodyBuilder collects a POST body (create) or a PATCH body with only the changed keys.
type bodyBuilder struct {
	body   map[string]any
	create bool
}

func newBody(create bool) *bodyBuilder { return &bodyBuilder{body: map[string]any{}, create: create} }

// changed: on create the value is set, on update it differs from the state. Unknown never counts.
func (b *bodyBuilder) changed(plan, prior attr.Value) bool {
	if plan.IsUnknown() {
		return false
	}
	if b.create {
		return !plan.IsNull()
	}
	return !plan.Equal(prior)
}

// differs: an update changes the value (create = false).
func (b *bodyBuilder) differs(plan, prior attr.Value) bool {
	return !b.create && b.changed(plan, prior)
}

func (b *bodyBuilder) put(key string, plan, prior attr.Value, v any) {
	if b.changed(plan, prior) {
		b.body[key] = v
	}
}

func (m *monitorBase) putBase(ctx context.Context, b *bodyBuilder, prior *monitorBase) diag.Diagnostics {
	var codes []string
	diags := m.AllowedCheckCountryCodes.ElementsAs(ctx, &codes, false)
	b.put("customerId", m.CustomerID, prior.CustomerID, m.CustomerID.ValueInt64())
	b.put("name", m.Name, prior.Name, m.Name.ValueString())
	b.put("hostname", m.Hostname, prior.Hostname, m.Hostname.ValueString())
	b.put("checkInterval", m.CheckInterval, prior.CheckInterval, m.CheckInterval.ValueInt64())
	b.put("timeoutSeconds", m.TimeoutSeconds, prior.TimeoutSeconds, m.TimeoutSeconds.ValueInt64())
	b.put("status", m.Status, prior.Status, m.Status.ValueString())
	b.put("managementType", m.ManagementType, prior.ManagementType, m.ManagementType.ValueString())
	b.put("allowedCheckCountryCodes", m.AllowedCheckCountryCodes, prior.AllowedCheckCountryCodes, codes) // nil = all locations
	return diags
}

func (m *monitorBase) fromRow(row *client.MonitorRow) {
	m.ID = types.StringValue(strconv.FormatInt(row.ID, 10))
	m.PublicID = types.StringValue(row.PublicID)
	m.OrganizationID = types.Int64Value(row.OrganizationID)
	m.CustomerID = types.Int64Value(row.CustomerID)
	m.Name = types.StringValue(row.Name)
	m.Hostname = types.StringValue(row.Hostname)
	m.CheckInterval = types.Int64Value(row.CheckInterval)
	m.TimeoutSeconds = types.Int64Value(row.TimeoutSeconds)
	m.Status = types.StringValue(row.Status)
	m.ManagementType = types.StringValue(row.ManagementType)
	m.AllowedCheckCountryCodes = stringSet(row.AllowedCheckCountryCodes)
	m.CreatedAt = types.StringValue(row.CreatedAt)
	m.UpdatedAt = types.StringValue(row.UpdatedAt)
}

func stringSet(v []string) types.Set {
	if len(v) == 0 {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, len(v))
	for i, s := range v {
		elems[i] = types.StringValue(s)
	}
	return types.SetValueMust(types.StringType, elems)
}

var noSpaces = regexp.MustCompile(`^\S+$`)

func monitorBaseAttrs(title string) map[string]schema.Attribute {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	return map[string]schema.Attribute{
		"id":              schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Numeric " + title + " id."},
		"public_id":       schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Public UUID of the " + title + "."},
		"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: keepI},
		"customer_id": schema.Int64Attribute{
			Required:    true,
			Description: "Customer the monitor belongs to. Changing it moves the monitor to another customer of the same organization in place.",
			Validators:  []validator.Int64{int64validator.AtLeast(1)},
		},
		"name": schema.StringAttribute{
			Required:    true,
			Description: "Display name, 1 to 255 characters.",
			Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
		},
		"hostname": schema.StringAttribute{
			Required:    true,
			Description: "Host name or IP address only: no scheme, no path, no spaces.",
			Validators:  []validator.String{stringvalidator.LengthBetween(1, 253), stringvalidator.RegexMatches(noSpaces, "must not contain spaces")},
		},
		"check_interval": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(30),
			Description: "Minutes between checks, 1 to 1440. Defaults to 30. The customer's package can require a larger minimum.",
			Validators:  []validator.Int64{int64validator.Between(1, 1440)},
		},
		"timeout_seconds": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(30),
			Description: "Timeout per check in seconds, 1 to 60. Defaults to 30.",
			Validators:  []validator.Int64{int64validator.Between(1, 60)},
		},
		"status": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("active"),
			Description: "`active`, `maintenance` or `disabled`. Defaults to `active`.",
			Validators:  []validator.String{stringvalidator.OneOf("active", "maintenance", "disabled")},
		},
		"management_type": schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			PlanModifiers: keepS,
			Description:   "`managed` or `self_service`. Only organization writers choose it; customer tokens always get `self_service`. Defaults to `managed`.",
			Validators:    []validator.String{stringvalidator.OneOf("managed", "self_service")},
		},
		"allowed_check_country_codes": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Description: "Upper-case ISO 3166-1 alpha-2 codes of the allowed check locations. Unset = all locations.",
			Validators: []validator.Set{
				setvalidator.SizeAtLeast(1),
				setvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Z]{2}$`), "must be an upper-case two-letter country code")),
			},
		},
		"created_at":      schema.StringAttribute{Computed: true, PlanModifiers: keepS},
		"updated_at":      schema.StringAttribute{Computed: true},
		"allow_duplicate": allowDuplicateAttr,
	}
}

// ---- resource ----------------------------------------------------------------------------------

type monitorResource[M any, P monitorModel[M]] struct {
	client *client.Client
	spec   monitorSpec
}

var (
	_ resource.ResourceWithConfigure   = &monitorResource[dnsMonitorModel, *dnsMonitorModel]{}
	_ resource.ResourceWithImportState = &monitorResource[dnsMonitorModel, *dnsMonitorModel]{}
)

func (r *monitorResource[M, P]) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.name
}

func (r *monitorResource[M, P]) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := monitorBaseAttrs(r.spec.title)
	maps.Copy(attrs, r.spec.attrs)
	resp.Schema = schema.Schema{
		Description: r.spec.description + " Import with the numeric id or the public id.",
		Attributes:  attrs,
	}
}

func (r *monitorResource[M, P]) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// fill copies an API row into m.
func fill[M any, P monitorModel[M]](ctx context.Context, m *M, row *client.MonitorRow) diag.Diagnostics {
	P(m).base().fromRow(row)
	return P(m).read(ctx, row)
}

func (r *monitorResource[M, P]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg, prior M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b := newBody(true)
	resp.Diagnostics.Append(P(&plan).base().putBase(ctx, b, P(&prior).base())...)
	resp.Diagnostics.Append(P(&plan).put(ctx, b, &cfg, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if P(&cfg).base().AllowDuplicate.ValueBool() {
		b.body["allowDuplicate"] = true
	}
	row, err := r.client.CreateMonitor(ctx, r.spec.kind, b.body)
	if err != nil {
		resp.Diagnostics.AddError("Creating "+r.spec.title+" failed", createErrorDetail(err, "uptimeify_"+r.spec.name))
		return
	}
	resp.Diagnostics.Append(fill[M, P](ctx, &plan, row)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource[M, P]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetMonitor(ctx, r.spec.kind, P(&state).base().ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading "+r.spec.title+" failed", err.Error())
		return
	}
	resp.Diagnostics.Append(fill[M, P](ctx, &state, row)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorResource[M, P]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg, state M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b := newBody(false)
	resp.Diagnostics.Append(P(&plan).base().putBase(ctx, b, P(&state).base())...)
	resp.Diagnostics.Append(P(&plan).put(ctx, b, &cfg, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := P(&state).base().ID.ValueString()
	var row *client.MonitorRow
	var err error
	if len(b.body) == 0 { // e.g. only a write-only value changed without its version
		row, err = r.client.GetMonitor(ctx, r.spec.kind, id)
	} else {
		row, err = r.client.UpdateMonitor(ctx, r.spec.kind, id, b.body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating "+r.spec.title+" failed", err.Error())
		return
	}
	resp.Diagnostics.Append(fill[M, P](ctx, &plan, row)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource[M, P]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteMonitor(ctx, r.spec.kind, P(&state).base().ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting "+r.spec.title+" failed", err.Error())
	}
}

func (r *monitorResource[M, P]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---- data source -------------------------------------------------------------------------------

// The data source has the resource's attributes, all computed, without the write-only ones and
// their *_wo_version triggers; `id` is the lookup key.
type monitorDataSource[M any, P monitorModel[M]] struct {
	client *client.Client
	spec   monitorSpec
}

var _ datasource.DataSourceWithConfigure = &monitorDataSource[dnsMonitorModel, *dnsMonitorModel]{}

func (d *monitorDataSource[M, P]) resourceSchema() schema.Schema {
	var resp resource.SchemaResponse
	(&monitorResource[M, P]{spec: d.spec}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func (d *monitorDataSource[M, P]) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.name
}

func (d *monitorDataSource[M, P]) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		Description: "Look up one " + d.spec.title + " by numeric id or public id. Secrets are never readable.",
		Attributes:  dataSourceAttrs(d.resourceSchema().Attributes),
	}
}

func dataSourceAttrs(in map[string]schema.Attribute) map[string]dsschema.Attribute {
	out := map[string]dsschema.Attribute{}
	for k, a := range in {
		if a.IsWriteOnly() || strings.HasSuffix(k, "_wo_version") {
			continue
		}
		desc := a.GetDescription()
		switch a := a.(type) {
		case schema.StringAttribute:
			out[k] = dsschema.StringAttribute{Computed: true, Description: desc}
		case schema.Int64Attribute:
			out[k] = dsschema.Int64Attribute{Computed: true, Description: desc}
		case schema.BoolAttribute:
			out[k] = dsschema.BoolAttribute{Computed: true, Description: desc}
		case schema.ListAttribute:
			out[k] = dsschema.ListAttribute{Computed: true, ElementType: a.ElementType, Description: desc}
		case schema.SetAttribute:
			out[k] = dsschema.SetAttribute{Computed: true, ElementType: a.ElementType, Description: desc}
		case schema.MapAttribute:
			out[k] = dsschema.MapAttribute{Computed: true, ElementType: a.ElementType, Description: desc}
		default:
			panic(fmt.Sprintf("dataSourceAttrs: unsupported attribute type %T for %s", a, k))
		}
	}
	out["id"] = dsschema.StringAttribute{Required: true, Description: "Numeric id or public id."}
	return out
}

func (d *monitorDataSource[M, P]) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *monitorDataSource[M, P]) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var id types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := d.client.GetMonitor(ctx, d.spec.kind, id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading "+d.spec.title+" failed", err.Error())
		return
	}
	var m M
	resp.Diagnostics.Append(fill[M, P](ctx, &m, row)...)
	full := tfsdk.State{Schema: d.resourceSchema()}
	resp.Diagnostics.Append(full.Set(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Project the resource-shaped value onto the data source schema.
	var vals map[string]tftypes.Value
	if err := full.Raw.As(&vals); err != nil {
		resp.Diagnostics.AddError("Reading "+d.spec.title+" failed", err.Error())
		return
	}
	dsType := resp.State.Schema.Type().TerraformType(ctx).(tftypes.Object)
	kept := map[string]tftypes.Value{}
	for k := range dsType.AttributeTypes {
		kept[k] = vals[k]
	}
	kept["id"] = tftypes.NewValue(tftypes.String, id.ValueString()) // keep a configured public id as written
	resp.State.Raw = tftypes.NewValue(dsType, kept)
}
