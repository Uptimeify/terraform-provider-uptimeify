package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &customerDataSource{}
	_ datasource.DataSourceWithConfigValidators = &customerDataSource{}
)

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
		Description: "Look up one customer by `id` or by `name` (exactly one). Fails when no or several visible customers carry the name.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Optional: true, Computed: true, Description: "Numeric id or public id. Use `tonumber(id)` for `customer_id` arguments."},
			"name":      schema.StringAttribute{Optional: true, Computed: true},
			"public_id": schema.StringAttribute{Computed: true},
			"status":    schema.StringAttribute{Computed: true, Description: "`active` or `inactive`."},
		},
	}
}

func (d *customerDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
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
	var found *client.Customer
	if !cfg.ID.IsNull() {
		c, err := d.client.GetCustomer(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading customer failed", err.Error())
			return
		}
		found = c
	} else {
		all, err := d.client.ListCustomers(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing customers failed", err.Error())
			return
		}
		var matches []client.Customer
		for _, c := range all {
			if c.Name == cfg.Name.ValueString() {
				matches = append(matches, c)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddError("Customer lookup by name failed",
				fmt.Sprintf("%d visible customers are named %q, expected exactly 1. Use id instead.", len(matches), cfg.Name.ValueString()))
			return
		}
		found = &matches[0]
	}
	if cfg.ID.IsNull() {
		cfg.ID = types.StringValue(strconv.FormatInt(found.ID, 10))
	}
	cfg.PublicID = types.StringValue(found.PublicID)
	cfg.Name = types.StringValue(found.Name)
	cfg.Status = types.StringValue(found.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
