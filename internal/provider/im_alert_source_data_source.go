package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &imAlertSourceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &imAlertSourceDataSource{}
)

type imAlertSourceDataSource struct{ client *client.Client }

func NewImAlertSourceDataSource() datasource.DataSource { return &imAlertSourceDataSource{} }

func (d *imAlertSourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_alert_source"
}

func (d *imAlertSourceDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&imAlertSourceResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema, imAlertSourceResourceOnly...)
	attrs["id"] = dschema.StringAttribute{Optional: true, Computed: true, Description: "Numeric source id."}
	attrs["name"] = dschema.StringAttribute{Optional: true, Computed: true}
	resp.Schema = dschema.Schema{
		Description: "Look up one Incident Management alert source by `id` or by `name` (exactly one). " +
			"Fails when no or several sources match the name. Tokens and secrets are never returned.",
		Attributes: attrs,
	}
}

func (d *imAlertSourceDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *imAlertSourceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imAlertSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imAlertSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.AlertSource
	if !cfg.ID.IsNull() {
		src, err := d.client.GetAlertSource(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading alert source failed", err.Error())
			return
		}
		found = src
	} else {
		all, err := d.client.ListAlertSources(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing alert sources failed", err.Error())
			return
		}
		var matches []client.AlertSource
		for _, s := range all {
			if s.Name == cfg.Name.ValueString() {
				matches = append(matches, s)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddError("Alert source lookup by name failed",
				fmt.Sprintf("%d alert sources are named %q, expected exactly 1. Use id instead.", len(matches), cfg.Name.ValueString()))
			return
		}
		found = &matches[0]
	}
	cfg.PayloadMapping = types.ObjectNull(payloadMappingAttrTypes())
	resp.Diagnostics.Append(cfg.fromAPI(ctx, found, true)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
