package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &monitoringLocationsDataSource{}

type monitoringLocationsDataSource struct{ client *client.Client }

type locationCountryModel struct {
	Code      types.String `tfsdk:"code"`
	Locations types.Int64  `tfsdk:"locations"`
	Workers   types.Int64  `tfsdk:"workers"`
}

type monitoringLocationsModel struct {
	ID           types.String           `tfsdk:"id"`
	CountryCodes []string               `tfsdk:"country_codes"`
	Countries    []locationCountryModel `tfsdk:"countries"`
}

func NewMonitoringLocationsDataSource() datasource.DataSource {
	return &monitoringLocationsDataSource{}
}

func (d *monitoringLocationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitoring_locations"
}

func (d *monitoringLocationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Countries with at least one active check worker. Use `country_codes` for `allowed_check_country_codes`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "Always `monitoring-locations`."},
			"country_codes": schema.ListAttribute{ElementType: types.StringType, Computed: true,
				Description: "ISO 3166-1 alpha-2 codes, sorted."},
			"countries": schema.ListNestedAttribute{Computed: true, Description: "Per country, sorted by code.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"code":      schema.StringAttribute{Computed: true, Description: "ISO 3166-1 alpha-2."},
					"locations": schema.Int64Attribute{Computed: true, Description: "Check locations in the country."},
					"workers":   schema.Int64Attribute{Computed: true, Description: "Active check workers in the country."},
				}}},
		},
	}
}

func (d *monitoringLocationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *monitoringLocationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	countries, err := d.client.ListLocationCountries(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading monitoring locations failed", err.Error())
		return
	}
	m := monitoringLocationsModel{ID: types.StringValue("monitoring-locations"), CountryCodes: []string{}, Countries: []locationCountryModel{}}
	for _, c := range countries {
		m.CountryCodes = append(m.CountryCodes, c.Code)
		m.Countries = append(m.Countries, locationCountryModel{Code: types.StringValue(c.Code),
			Locations: types.Int64Value(c.Locations), Workers: types.Int64Value(c.Workers)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
