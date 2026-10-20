package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &customFieldDataSource{}
	_ datasource.DataSourceWithConfigValidators = &customFieldDataSource{}
)

type customFieldDataSource struct{ client *client.Client }

func NewCustomFieldDataSource() datasource.DataSource { return &customFieldDataSource{} }

func (d *customFieldDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field"
}

func (d *customFieldDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&customFieldResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema)
	attrs["id"] = dschema.StringAttribute{Optional: true, Computed: true, Description: "Numeric field id."}
	attrs["field_key"] = dschema.StringAttribute{Optional: true, Computed: true}
	resp.Schema = dschema.Schema{
		Description: "Look up one active custom field by `id` or by `field_key` (exactly one).",
		Attributes:  attrs,
	}
}

func (d *customFieldDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("field_key"))}
}

func (d *customFieldDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *customFieldDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg customFieldModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.CustomField
	if !cfg.ID.IsNull() {
		f, err := d.client.GetCustomField(ctx, cfg.ID.ValueString())
		if err == nil && !f.IsActive {
			err = fmt.Errorf("custom field %s is deactivated", cfg.ID.ValueString())
		}
		if err != nil {
			resp.Diagnostics.AddError("Reading custom field failed", err.Error())
			return
		}
		found = f
	} else {
		all, err := d.client.ListCustomFields(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing custom fields failed", err.Error())
			return
		}
		for i := range all {
			if all[i].FieldKey == cfg.FieldKey.ValueString() {
				found = &all[i]
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("Custom field not found", fmt.Sprintf("no active custom field with field_key %q", cfg.FieldKey.ValueString()))
			return
		}
	}
	resp.Diagnostics.Append(cfg.fromAPI(ctx, found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
