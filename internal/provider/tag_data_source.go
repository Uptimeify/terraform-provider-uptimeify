package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &tagDataSource{}
	_ datasource.DataSourceWithConfigValidators = &tagDataSource{}
)

type tagDataSource struct{ client *client.Client }

func NewTagDataSource() datasource.DataSource { return &tagDataSource{} }

func (d *tagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (d *tagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up one tag by `id` or by `name` (exactly one). Fails when no or several visible tags match the name.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Numeric id or public id."},
			"name":        schema.StringAttribute{Optional: true, Computed: true},
			"public_id":   schema.StringAttribute{Computed: true},
			"color":       schema.StringAttribute{Computed: true},
			"customer_id": schema.Int64Attribute{Computed: true},
			"created_at":  schema.StringAttribute{Computed: true},
			"updated_at":  schema.StringAttribute{Computed: true},
		},
	}
}

func (d *tagDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *tagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *tagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg tagModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var found *client.Tag
	if !cfg.ID.IsNull() {
		tag, err := d.client.GetTag(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading tag failed", err.Error())
			return
		}
		found = tag
	} else {
		tags, err := d.client.ListTags(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing tags failed", err.Error())
			return
		}
		name := cfg.Name.ValueString()
		var matches []client.Tag
		for _, t := range tags {
			if t.Name == name {
				matches = append(matches, t)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddError("Tag lookup by name failed",
				fmt.Sprintf("%d visible tags are named %q, expected exactly 1. Use id instead.", len(matches), name))
			return
		}
		found = &matches[0]
	}

	configuredID := cfg.ID
	cfg.fromAPI(found)
	if !configuredID.IsNull() {
		cfg.ID = configuredID // keep a configured public id as written
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
