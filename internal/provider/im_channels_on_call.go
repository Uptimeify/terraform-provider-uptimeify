package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var _ datasource.DataSourceWithConfigure = &imOnCallDataSource{}

type imOnCallDataSource struct{ client *client.Client }

type imOnCallEntryModel struct {
	TeamID      types.Int64  `tfsdk:"team_id"`
	ScheduleID  types.Int64  `tfsdk:"schedule_id"`
	UserID      types.String `tfsdk:"user_id"`
	UserName    types.String `tfsdk:"user_name"`
	UserImage   types.String `tfsdk:"user_image"`
	ShiftEndsAt types.String `tfsdk:"shift_ends_at"`
}

type imOnCallUserModel struct {
	UserID    types.String `tfsdk:"user_id"`
	UserName  types.String `tfsdk:"user_name"`
	UserImage types.String `tfsdk:"user_image"`
}

type imOnCallTierModel struct {
	TierID         types.Int64         `tfsdk:"tier_id"`
	TierOrder      types.Int64         `tfsdk:"tier_order"`
	Users          []imOnCallUserModel `tfsdk:"users"`
	UncoveredSince types.String        `tfsdk:"uncovered_since"`
}

type imOnCallModel struct {
	TeamID     types.Int64          `tfsdk:"team_id"`
	ScheduleID types.Int64          `tfsdk:"schedule_id"`
	Entries    []imOnCallEntryModel `tfsdk:"entries"`
	Tiers      []imOnCallTierModel  `tfsdk:"tiers"`
}

func NewImOnCallDataSource() datasource.DataSource { return &imOnCallDataSource{} }

func (d *imOnCallDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_on_call"
}

func (d *imOnCallDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) dschema.StringAttribute {
		return dschema.StringAttribute{Computed: true, Description: desc}
	}
	user := map[string]dschema.Attribute{
		"user_id":    str("User id."),
		"user_name":  str("Display name, may be null."),
		"user_image": str("Avatar URL, may be null."),
	}
	entry := map[string]dschema.Attribute{
		"team_id":       dschema.Int64Attribute{Computed: true},
		"schedule_id":   dschema.Int64Attribute{Computed: true},
		"shift_ends_at": str("End of the current shift (RFC 3339)."),
	}
	for k, v := range user {
		entry[k] = v
	}
	resp.Schema = dschema.Schema{
		Description: "Who is on call right now (read at plan time, so it changes between runs). Incident Management " +
			"must be active for the organization.",
		Attributes: map[string]dschema.Attribute{
			"team_id": dschema.Int64Attribute{Optional: true, Description: "Only this team; also fills `tiers`.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"schedule_id": dschema.Int64Attribute{Optional: true, Description: "Only this schedule.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"entries": dschema.ListNestedAttribute{Computed: true,
				Description:  "One entry per schedule with someone on call now (`GET /api/im/on-call`), after the filters.",
				NestedObject: dschema.NestedAttributeObject{Attributes: entry}},
			"tiers": dschema.ListNestedAttribute{Computed: true,
				Description: "With `team_id`: one entry per escalation tier of the team with everyone on call now " +
					"(`GET /api/im/teams/{id}/on-call-now`), overrides applied. Null without `team_id`.",
				NestedObject: dschema.NestedAttributeObject{Attributes: map[string]dschema.Attribute{
					"tier_id":         dschema.Int64Attribute{Computed: true},
					"tier_order":      dschema.Int64Attribute{Computed: true},
					"users":           dschema.ListNestedAttribute{Computed: true, NestedObject: dschema.NestedAttributeObject{Attributes: user}},
					"uncovered_since": str("Since when nobody covers the tier; null when covered or not within 14 days."),
				}}},
		},
	}
}

func (d *imOnCallDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imOnCallDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m imOnCallModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := d.client.ListImOnCall(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading on-call failed", err.Error())
		return
	}
	m.Entries = []imOnCallEntryModel{}
	for _, r := range rows {
		if (!m.TeamID.IsNull() && r.TeamID != m.TeamID.ValueInt64()) || (!m.ScheduleID.IsNull() && r.ScheduleID != m.ScheduleID.ValueInt64()) {
			continue
		}
		m.Entries = append(m.Entries, imOnCallEntryModel{TeamID: types.Int64Value(r.TeamID), ScheduleID: types.Int64Value(r.ScheduleID),
			UserID: types.StringValue(r.UserID), UserName: types.StringPointerValue(r.UserName),
			UserImage: types.StringPointerValue(r.UserImage), ShiftEndsAt: types.StringValue(r.ShiftEndsAt)})
	}
	if !m.TeamID.IsNull() {
		tiers, err := d.client.ListImTeamOnCallNow(ctx, m.TeamID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Reading team on-call failed", err.Error())
			return
		}
		m.Tiers = []imOnCallTierModel{}
		for _, t := range tiers {
			users := []imOnCallUserModel{}
			for _, u := range t.Users {
				users = append(users, imOnCallUserModel{UserID: types.StringValue(u.UserID),
					UserName: types.StringPointerValue(u.UserName), UserImage: types.StringPointerValue(u.UserImage)})
			}
			m.Tiers = append(m.Tiers, imOnCallTierModel{TierID: types.Int64Value(t.TierID), TierOrder: types.Int64Value(t.TierOrder),
				Users: users, UncoveredSince: types.StringPointerValue(t.UncoveredSince)})
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
