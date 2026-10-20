package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &userDataSource{}

type userDataSource struct{ client *client.Client }

type userModel struct {
	ID       types.String `tfsdk:"id"`
	Email    types.String `tfsdk:"email"`
	Name     types.String `tfsdk:"name"`
	Role     types.String `tfsdk:"role"`
	IsActive types.Bool   `tfsdk:"is_active"`
}

func NewUserDataSource() datasource.DataSource { return &userDataSource{} }

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up one login of the organization by email (case-insensitive), e.g. for team members and rotations.",
		Attributes: map[string]schema.Attribute{
			"email":     schema.StringAttribute{Required: true},
			"id":        schema.StringAttribute{Computed: true, Description: "User id, as `user_id` in IM resources."},
			"name":      schema.StringAttribute{Computed: true},
			"role":      schema.StringAttribute{Computed: true, Description: "`admin`, `editor`, `readonly` or `responder`. IM accepts admin, editor and responder."},
			"is_active": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg userModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	u, err := d.client.FindUserByEmail(ctx, cfg.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("User lookup failed", err.Error())
		return
	}
	cfg.ID = types.StringValue(u.ID)
	cfg.Name = types.StringPointerValue(u.Name)
	cfg.Role = types.StringValue(u.Role)
	cfg.IsActive = types.BoolValue(u.IsActive)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
