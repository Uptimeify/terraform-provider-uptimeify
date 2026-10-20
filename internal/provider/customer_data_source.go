package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &customerDataSource{}

type customerDataSource struct{ client *client.Client }

type customerModel struct {
	ID       types.String `tfsdk:"id"`
	PublicID types.String `tfsdk:"public_id"`
	Name     types.String `tfsdk:"name"`
	Status   types.String `tfsdk:"status"`
}

func NewCustomerDataSource() datasource.DataSource { return &customerDataSource{} }

func (d *customerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer"
}

func (d *customerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up one customer by exact name. Fails when no or several visible customers have that name.",
		Attributes: map[string]schema.Attribute{
			"name":      schema.StringAttribute{Required: true},
			"id":        schema.StringAttribute{Computed: true, Description: "Numeric customer id, e.g. for `customer_id` of a monitor."},
			"public_id": schema.StringAttribute{Computed: true},
			"status":    schema.StringAttribute{Computed: true, Description: "`active` or `inactive`."},
		},
	}
}

func (d *customerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *customerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg customerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	customers, err := d.client.ListCustomers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing customers failed", err.Error())
		return
	}
	name := cfg.Name.ValueString()
	var matches []client.Customer
	for _, c := range customers {
		if c.Name == name {
			matches = append(matches, c)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Customer lookup by name failed",
			fmt.Sprintf("%d visible customers are named %q, expected exactly 1.", len(matches), name))
		return
	}
	c := matches[0]
	cfg.ID = types.StringValue(strconv.FormatInt(c.ID, 10))
	cfg.PublicID = types.StringValue(c.PublicID)
	cfg.Status = types.StringValue(c.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
