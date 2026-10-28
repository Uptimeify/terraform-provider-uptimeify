package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &websiteDataSource{}

type websiteDataSource struct{ client *client.Client }

func NewWebsiteDataSource() datasource.DataSource { return &websiteDataSource{} }

func (d *websiteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website"
}

// Schema mirrors the resource's readable attributes as computed ones, so both stay in step.
func (d *websiteDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&websiteResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := map[string]dschema.Attribute{}
	for name, a := range rs.Schema.Attributes {
		if a.IsWriteOnly() || strings.HasSuffix(name, "_wo_version") {
			continue
		}
		desc := a.GetDescription()
		switch t := a.(type) {
		case rschema.StringAttribute:
			attrs[name] = dschema.StringAttribute{Computed: true, Sensitive: t.Sensitive, Description: desc}
		case rschema.Int64Attribute:
			attrs[name] = dschema.Int64Attribute{Computed: true, Description: desc}
		case rschema.BoolAttribute:
			attrs[name] = dschema.BoolAttribute{Computed: true, Description: desc}
		case rschema.SetAttribute:
			attrs[name] = dschema.SetAttribute{Computed: true, ElementType: t.ElementType, Description: desc}
		}
	}
	attrs["id"] = dschema.StringAttribute{Required: true, Description: "Numeric id or public id."}
	resp.Schema = dschema.Schema{
		Description: "Read one website monitor by id. Secrets (headers, body, auth secrets) are never returned.",
		Attributes:  attrs,
	}
}

func (d *websiteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *websiteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m websiteModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	configuredID := m.ID
	w, err := d.client.GetWebsite(ctx, configuredID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading website failed", err.Error())
		return
	}
	resp.Diagnostics.Append(m.fromAPI(ctx, w)...)
	m.ID = configuredID // keep a configured public id as written
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
