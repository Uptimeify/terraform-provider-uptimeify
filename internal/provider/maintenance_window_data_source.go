package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &maintenanceWindowDataSource{}

type maintenanceWindowDataSource struct{ client *client.Client }

func NewMaintenanceWindowDataSource() datasource.DataSource { return &maintenanceWindowDataSource{} }

func (d *maintenanceWindowDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (d *maintenanceWindowDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "Look up one maintenance window by `id`.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true, Description: "Numeric id or public id."},
			"public_id":   c(""),
			"name":        c(""),
			"description": c(""),
			"start_time":  c("UTC, RFC3339."),
			"end_time":    c("UTC, RFC3339."),
			"timezone":    c("IANA zone the recurrence is read in."),
			"is_active":   schema.BoolAttribute{Computed: true},
			"customer_id": schema.Int64Attribute{Computed: true, Description: "Set only for a whole-customer window (no targets, no tags)."},
			"targets": schema.SetNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"type": c(""),
				"id":   schema.Int64Attribute{Computed: true},
			}}},
			"tag_ids": schema.SetAttribute{ElementType: types.Int64Type, Computed: true},
			"recurrence_pattern": schema.SingleNestedAttribute{Computed: true, Attributes: map[string]schema.Attribute{
				"frequency":           c(""),
				"interval_count":      schema.Int64Attribute{Computed: true},
				"days_of_week":        schema.ListAttribute{ElementType: types.Int64Type, Computed: true},
				"day_of_month":        schema.Int64Attribute{Computed: true},
				"end_recurrence_date": c(""),
			}},
			"created_at": c(""),
			"updated_at": c(""),
		},
	}
}

func (d *maintenanceWindowDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *maintenanceWindowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg maintenanceWindowModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := d.client.GetMaintenanceWindow(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading maintenance window failed", err.Error())
		return
	}
	configuredID := cfg.ID
	cfg.fromAPI(w, &maintenanceWindowModel{})
	cfg.ID = configuredID // keep a configured public id as written
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
