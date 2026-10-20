package provider

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Terraform models of payloadMapping (im-sources.ts). Slices/maps: nil = null = absent in the
// API, non-nil (even empty) = sent and read back as is, so the verbatim stored mapping round-trips.

type pmEntryModel struct {
	From types.String `tfsdk:"from"`
	To   types.String `tfsdk:"to"`
}

type pmStepModel struct {
	Kind     types.String   `tfsdk:"kind"`
	Type     types.String   `tfsdk:"type"`
	Expr     types.String   `tfsdk:"expr"`
	Entries  []pmEntryModel `tfsdk:"entries"`
	Fallback types.String   `tfsdk:"fallback"`
}

type pmFlagsModel struct {
	Grouped types.Bool `tfsdk:"grouped"`
	Hidden  types.Bool `tfsdk:"hidden"`
	Image   types.Bool `tfsdk:"image"`
	Expand  types.Bool `tfsdk:"expand"`
}

type pmAttributeModel struct {
	Key      types.String  `tfsdk:"key"`
	Standard types.Bool    `tfsdk:"standard"`
	Steps    []pmStepModel `tfsdk:"steps"`
	Flags    *pmFlagsModel `tfsdk:"flags"`
}

type payloadMappingModel struct {
	Attributes      []pmAttributeModel `tfsdk:"attributes"`
	Title           types.String       `tfsdk:"title"`
	Severity        types.String       `tfsdk:"severity"`
	Status          types.String       `tfsdk:"status"`
	Host            types.String       `tfsdk:"host"`
	URL             types.String       `tfsdk:"url"`
	DedupKey        types.String       `tfsdk:"dedup_key"`
	SeverityMap     map[string]string  `tfsdk:"severity_map"`
	ResolveValues   []string           `tfsdk:"resolve_values"`
	Custom          map[string]string  `tfsdk:"custom"`
	DefaultSeverity types.String       `tfsdk:"default_severity"`
	EmailSelectors  map[string]string  `tfsdk:"email_selectors"`
}

func ptrSlice[T any](s []T) *[]T {
	if s == nil {
		return nil
	}
	return &s
}

func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	if *p == nil {
		return []T{}
	}
	return *p
}

func ptrMap(m map[string]string) *map[string]string {
	if m == nil {
		return nil
	}
	return &m
}

func derefMap(p *map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	if *p == nil {
		return map[string]string{}
	}
	return *p
}

func (m *payloadMappingModel) toAPI() client.PayloadMapping {
	out := client.PayloadMapping{
		Title:           m.Title.ValueStringPointer(),
		Severity:        m.Severity.ValueStringPointer(),
		Status:          m.Status.ValueStringPointer(),
		Host:            m.Host.ValueStringPointer(),
		URL:             m.URL.ValueStringPointer(),
		DedupKey:        m.DedupKey.ValueStringPointer(),
		SeverityMap:     ptrMap(m.SeverityMap),
		ResolveValues:   ptrSlice(m.ResolveValues),
		Custom:          ptrMap(m.Custom),
		DefaultSeverity: m.DefaultSeverity.ValueStringPointer(),
		EmailSelectors:  ptrMap(m.EmailSelectors),
	}
	if m.Attributes != nil {
		v2 := 2
		out.Version = &v2
		attrs := []client.PMAttribute{}
		for _, a := range m.Attributes {
			ca := client.PMAttribute{Key: a.Key.ValueString(), Standard: a.Standard.ValueBoolPointer()}
			if a.Steps != nil {
				steps := []client.PMStep{}
				for _, s := range a.Steps {
					cs := client.PMStep{Kind: s.Kind.ValueString(), Type: s.Type.ValueStringPointer(),
						Expr: s.Expr.ValueStringPointer(), Fallback: s.Fallback.ValueStringPointer()}
					if s.Entries != nil {
						entries := []client.PMEntry{}
						for _, e := range s.Entries {
							entries = append(entries, client.PMEntry{From: e.From.ValueString(), To: e.To.ValueString()})
						}
						cs.Entries = &entries
					} else if cs.Kind == "valueMap" {
						cs.Entries = &[]client.PMEntry{} // the API requires the array
					}
					steps = append(steps, cs)
				}
				ca.Steps = &steps
			}
			if f := a.Flags; f != nil {
				ca.Flags = &client.PMFlags{Grouped: f.Grouped.ValueBoolPointer(), Hidden: f.Hidden.ValueBoolPointer(),
					Image: f.Image.ValueBoolPointer(), Expand: f.Expand.ValueBoolPointer()}
			}
			attrs = append(attrs, ca)
		}
		out.Attributes = &attrs
	}
	return out
}

// payloadMappingFromAPI returns nil for an empty stored mapping ({}).
func payloadMappingFromAPI(p client.PayloadMapping) *payloadMappingModel {
	m := &payloadMappingModel{
		Title:           types.StringPointerValue(p.Title),
		Severity:        types.StringPointerValue(p.Severity),
		Status:          types.StringPointerValue(p.Status),
		Host:            types.StringPointerValue(p.Host),
		URL:             types.StringPointerValue(p.URL),
		DedupKey:        types.StringPointerValue(p.DedupKey),
		SeverityMap:     derefMap(p.SeverityMap),
		ResolveValues:   derefSlice(p.ResolveValues),
		Custom:          derefMap(p.Custom),
		DefaultSeverity: types.StringPointerValue(p.DefaultSeverity),
		EmailSelectors:  derefMap(p.EmailSelectors),
	}
	if p.Attributes != nil {
		m.Attributes = []pmAttributeModel{}
		for _, a := range *p.Attributes {
			ma := pmAttributeModel{Key: types.StringValue(a.Key), Standard: types.BoolPointerValue(a.Standard)}
			if a.Steps != nil {
				ma.Steps = []pmStepModel{}
				for _, s := range *a.Steps {
					ms := pmStepModel{Kind: types.StringValue(s.Kind), Type: types.StringPointerValue(s.Type),
						Expr: types.StringPointerValue(s.Expr), Fallback: types.StringPointerValue(s.Fallback)}
					if s.Entries != nil {
						ms.Entries = []pmEntryModel{}
						for _, e := range *s.Entries {
							ms.Entries = append(ms.Entries, pmEntryModel{From: types.StringValue(e.From), To: types.StringValue(e.To)})
						}
					}
					ma.Steps = append(ma.Steps, ms)
				}
			}
			if f := a.Flags; f != nil {
				ma.Flags = &pmFlagsModel{Grouped: types.BoolPointerValue(f.Grouped), Hidden: types.BoolPointerValue(f.Hidden),
					Image: types.BoolPointerValue(f.Image), Expand: types.BoolPointerValue(f.Expand)}
			}
			m.Attributes = append(m.Attributes, ma)
		}
	}
	if m.isEmpty() {
		return nil
	}
	return m
}

func (m *payloadMappingModel) isEmpty() bool {
	return m.Attributes == nil && m.Title.IsNull() && m.Severity.IsNull() && m.Status.IsNull() && m.Host.IsNull() &&
		m.URL.IsNull() && m.DedupKey.IsNull() && m.SeverityMap == nil && m.ResolveValues == nil &&
		m.Custom == nil && m.DefaultSeverity.IsNull() && m.EmailSelectors == nil
}

var sevValidator = []validator.String{stringvalidator.OneOf(client.ImSeverities...)}

func payloadMappingSchema() schema.SingleNestedAttribute {
	optS := func(d string) schema.StringAttribute { return schema.StringAttribute{Optional: true, Description: d} }
	optB := func() schema.BoolAttribute { return schema.BoolAttribute{Optional: true} }
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "How a vendor payload becomes an alert. Either the v2 `attributes` list or the v1 selector fields " +
			"(`title`, `severity`, `status`, ...). Unset = not managed: the server keeps its mapping (a preset type starts " +
			"with the preset mapping; read it with the data source); removing the block later keeps the last mapping. " +
			"An import reads the stored mapping. Must stay unset for `type = \"email\"` (the server builds that mapping).",
		Attributes: map[string]schema.Attribute{
			"attributes": schema.ListNestedAttribute{
				Optional:    true,
				Description: "v2 attribute-list mapping: ordered attributes, each with a step chain.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key":      schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
					"standard": schema.BoolAttribute{Optional: true, Description: "A standard alert field (title, severity, status, ...)."},
					"steps": schema.ListNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"kind": schema.StringAttribute{Required: true, Description: "`extract` or `valueMap`.",
								Validators: []validator.String{stringvalidator.OneOf("extract", "valueMap")}},
							"type": schema.StringAttribute{Optional: true, Description: "extract: `path`, `jsonpath` or `constant`.",
								Validators: []validator.String{stringvalidator.OneOf("path", "jsonpath", "constant")}},
							"expr": optS("extract: the expression."),
							"entries": schema.ListNestedAttribute{
								Optional:    true,
								Description: "valueMap: from -> to pairs.",
								NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
									"from": schema.StringAttribute{Required: true},
									"to":   schema.StringAttribute{Required: true},
								}},
							},
							"fallback": optS("valueMap: value when no entry matches."),
						}},
					},
					"flags": schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"grouped": optB(), "hidden": optB(), "image": optB(), "expand": optB(),
						},
					},
				}},
			},
			"title":     optS("v1: selector of the alert title."),
			"severity":  optS("v1: selector of the vendor severity."),
			"status":    optS("v1: selector of the vendor status."),
			"host":      optS("v1: selector of the host."),
			"url":       optS("v1: selector of the URL."),
			"dedup_key": optS("v1: selector of the dedup key."),
			"severity_map": schema.MapAttribute{ElementType: types.StringType, Optional: true,
				Description: "v1: vendor value -> `sev1`..`sev4`.",
				Validators:  []validator.Map{mapvalidator.ValueStringsAre(sevValidator...)}},
			"resolve_values": schema.ListAttribute{ElementType: types.StringType, Optional: true,
				Description: "v1: status values that resolve the alert."},
			"custom": schema.MapAttribute{ElementType: types.StringType, Optional: true,
				Description: "v1: extra field -> selector."},
			"default_severity": schema.StringAttribute{Optional: true, Validators: sevValidator,
				Description: "Severity when the mapping yields none. The server treats unset as `sev3`."},
			"email_selectors": schema.MapAttribute{ElementType: types.StringType, Optional: true,
				Description: "email sources: field -> email selector. The server builds it on create; read here."},
		},
	}
}

// maintenance windows of a source

type imMaintenanceWindowModel struct {
	DaysOfWeek []int64      `tfsdk:"days_of_week"`
	From       types.String `tfsdk:"from"`
	To         types.String `tfsdk:"to"`
	Timezone   types.String `tfsdk:"timezone"`
}

func maintenanceToAPI(ws []imMaintenanceWindowModel) *client.SourceMaintenance {
	if ws == nil {
		return nil
	}
	out := &client.SourceMaintenance{Windows: []client.MaintenanceWindowIM{}}
	for _, w := range ws {
		out.Windows = append(out.Windows, client.MaintenanceWindowIM{Dow: ptrSlice(w.DaysOfWeek), From: w.From.ValueString(),
			To: w.To.ValueString(), Timezone: w.Timezone.ValueStringPointer()})
	}
	return out
}

func maintenanceFromAPI(m *client.SourceMaintenance) []imMaintenanceWindowModel {
	if m == nil {
		return nil
	}
	out := []imMaintenanceWindowModel{}
	for _, w := range m.Windows {
		out = append(out, imMaintenanceWindowModel{DaysOfWeek: derefSlice(w.Dow), From: types.StringValue(w.From),
			To: types.StringValue(w.To), Timezone: types.StringPointerValue(w.Timezone)})
	}
	return out
}

// toDataSourceAttribute turns a resource attribute into a computed data source attribute,
// nested ones included, so data sources mirror the resource schema.
func toDataSourceAttribute(a schema.Attribute) dschema.Attribute {
	d := a.GetDescription()
	nested := func(attrs map[string]schema.Attribute) map[string]dschema.Attribute {
		out := map[string]dschema.Attribute{}
		for k, v := range attrs {
			out[k] = toDataSourceAttribute(v)
		}
		return out
	}
	switch t := a.(type) {
	case schema.StringAttribute:
		return dschema.StringAttribute{Computed: true, Description: d, Sensitive: t.Sensitive}
	case schema.Int64Attribute:
		return dschema.Int64Attribute{Computed: true, Description: d}
	case schema.BoolAttribute:
		return dschema.BoolAttribute{Computed: true, Description: d}
	case schema.ListAttribute:
		return dschema.ListAttribute{Computed: true, ElementType: t.ElementType, Description: d}
	case schema.MapAttribute:
		return dschema.MapAttribute{Computed: true, ElementType: t.ElementType, Description: d}
	case schema.SingleNestedAttribute:
		return dschema.SingleNestedAttribute{Computed: true, Description: d, Attributes: nested(t.Attributes)}
	case schema.ListNestedAttribute:
		return dschema.ListNestedAttribute{Computed: true, Description: d,
			NestedObject: dschema.NestedAttributeObject{Attributes: nested(t.NestedObject.Attributes)}}
	}
	panic("toDataSourceAttribute: unhandled attribute type")
}

// dataSourceAttributes mirrors rs except the skipped names.
func dataSourceAttributes(rs schema.Schema, skip ...string) map[string]dschema.Attribute {
	out := map[string]dschema.Attribute{}
	for k, a := range rs.Attributes {
		if a.IsWriteOnly() {
			continue
		}
		skipped := false
		for _, s := range skip {
			skipped = skipped || s == k
		}
		if !skipped {
			out[k] = toDataSourceAttribute(a)
		}
	}
	return out
}

// jsonEqual compares the JSON the API would receive.
func jsonEqual(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}
