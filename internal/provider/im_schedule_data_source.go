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
	_ datasource.DataSourceWithConfigure        = &imScheduleDataSource{}
	_ datasource.DataSourceWithConfigValidators = &imScheduleDataSource{}
)

type imScheduleDataSource struct{ client *client.Client }

// imScheduleDataModel is the resource model plus what only the read side shows.
type imScheduleDataModel struct {
	imScheduleModel
	TeamName       types.String `tfsdk:"team_name"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func NewIMScheduleDataSource() datasource.DataSource { return &imScheduleDataSource{} }

func (d *imScheduleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_schedule"
}

func (d *imScheduleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func() schema.Attribute { return schema.StringAttribute{Computed: true} }
	n := func() schema.Attribute { return schema.Int64Attribute{Computed: true} }
	resp.Schema = schema.Schema{
		Description: "Look up one on-call schedule by `id` or by `name` (exactly one). The default schedules of different teams can share a name " +
			"(`Tier 1 schedule`), so `team_id` narrows a lookup by name. Fails when no or several schedules match.",
		Attributes: map[string]schema.Attribute{
			"id":                      schema.StringAttribute{Optional: true, Computed: true, Description: "Numeric schedule id."},
			"name":                    schema.StringAttribute{Optional: true, Computed: true},
			"team_id":                 schema.Int64Attribute{Optional: true, Computed: true, Description: "Team of the schedule; narrows a lookup by `name`, conflicts with `id`."},
			"team_name":               c(),
			"tier_id":                 n(),
			"organization_id":         n(),
			"timezone":                c(),
			"effective_from":          c(),
			"effective_until":         c(),
			"rotation_mode":           c(),
			"rotation_repeats":        c(),
			"custom_repeat_unit":      c(),
			"custom_repeat_value":     n(),
			"auto_rotation_size":      n(),
			"round_robin_size":        n(),
			"starts_on_day_of_week":   n(),
			"starts_on_date_of_month": n(),
			"starts_on_time":          c(),
			"created_at":              c(),
			"updated_at":              c(),
			"weekly_schedules": schema.ListNestedAttribute{Computed: true, Description: "Weekly windows; empty = around the clock.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"from":  c(),
					"until": c(),
					"days":  schema.ListAttribute{ElementType: types.Int64Type, Computed: true, Description: "ISO weekdays, 1 = Monday to 7 = Sunday."},
				}}},
			"rotations": schema.ListNestedAttribute{Computed: true, Description: "Ordered rotation groups.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"members": schema.ListAttribute{ElementType: types.StringType, Computed: true, Description: "User ids in order."},
				}}},
		},
	}
}

func (d *imScheduleDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.Conflicting(path.MatchRoot("id"), path.MatchRoot("team_id")),
	}
}

func (d *imScheduleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

// findIMSchedule returns the id of the one schedule with this name; teamID 0 = any team.
func findIMSchedule(items []client.IMScheduleListItem, name string, teamID int64) (int64, error) {
	var id int64
	hits := 0
	for _, s := range items {
		if s.Name == name && (teamID == 0 || s.TeamID == teamID) {
			id, hits = s.ID, hits+1
		}
	}
	if hits != 1 {
		return 0, fmt.Errorf("%d schedules are named %q, expected exactly 1. Use id or team_id instead.", hits, name)
	}
	return id, nil
}

func (d *imScheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imScheduleDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var id int64
	if !cfg.ID.IsNull() {
		n, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("Invalid id", "id must be the numeric schedule id")
			return
		}
		id = n
	} else {
		items, err := d.client.ListIMSchedules(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing IM schedules failed", err.Error())
			return
		}
		if id, err = findIMSchedule(items, cfg.Name.ValueString(), cfg.TeamID.ValueInt64()); err != nil {
			resp.Diagnostics.AddError("IM schedule lookup by name failed", err.Error())
			return
		}
	}
	s, err := d.client.GetIMSchedule(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading IM schedule failed", err.Error())
		return
	}
	cfg.fromAPI(s)
	cfg.TeamName = types.StringNull()
	if s.Team != nil {
		cfg.TeamName = types.StringValue(s.Team.Name)
	}
	cfg.OrganizationID = types.Int64Value(s.OrganizationID)
	cfg.CreatedAt = types.StringValue(s.CreatedAt)
	cfg.UpdatedAt = types.StringValue(s.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
