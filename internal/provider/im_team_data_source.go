package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &imTeamDataSource{}
	_ datasource.DataSourceWithConfigValidators = &imTeamDataSource{}
)

type imTeamDataSource struct{ client *client.Client }

func NewIMTeamDataSource() datasource.DataSource { return &imTeamDataSource{} }

func (d *imTeamDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_team"
}

func (d *imTeamDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func() schema.Attribute { return schema.StringAttribute{Computed: true} }
	n := func() schema.Attribute { return schema.Int64Attribute{Computed: true} }
	b := func() schema.Attribute { return schema.BoolAttribute{Computed: true} }
	resp.Schema = schema.Schema{
		Description: "Look up one Incident Management team by `id` or by `name` (exactly one). Fails when no or several teams match the name.",
		Attributes: map[string]schema.Attribute{
			"id":                               schema.StringAttribute{Optional: true, Computed: true, Description: "Numeric team id."},
			"name":                             schema.StringAttribute{Optional: true, Computed: true},
			"label_color":                      c(),
			"analytics_enabled":                b(),
			"timezone":                         c(),
			"escalation_repeats":               n(),
			"escalation_repeats_after_minutes": n(),
			"escalation_repeats_stop_mode":     c(),
			"engagement_report_enabled":        b(),
			"engagement_report_day_of_week":    n(),
			"engagement_report_time":           c(),
			"organization_id":                  n(),
			"created_at":                       c(),
		},
	}
}

func (d *imTeamDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *imTeamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imTeamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imTeamModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var id int64
	if !cfg.ID.IsNull() {
		n, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("Invalid id", "id must be the numeric team id")
			return
		}
		id = n
	} else {
		teams, err := d.client.ListIMTeams(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing IM teams failed", err.Error())
			return
		}
		name, hits := cfg.Name.ValueString(), 0
		for _, t := range teams {
			if t.Name == name {
				id, hits = t.ID, hits+1
			}
		}
		if hits != 1 {
			resp.Diagnostics.AddError("IM team lookup by name failed",
				fmt.Sprintf("%d teams are named %q, expected exactly 1. Use id instead.", hits, name))
			return
		}
	}
	t, err := d.client.GetIMTeam(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading IM team failed", err.Error())
		return
	}
	cfg.fromAPI(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
