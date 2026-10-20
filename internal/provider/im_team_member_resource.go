package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure        = &imTeamMemberResource{}
	_ resource.ResourceWithImportState      = &imTeamMemberResource{}
	_ resource.ResourceWithConfigValidators = &imTeamMemberResource{}
)

var imRoles = []string{"admin", "member", "stakeholder"}

type imTeamMemberResource struct{ client *client.Client }

type imTeamMemberModel struct {
	ID          types.String `tfsdk:"id"`
	TeamID      types.Int64  `tfsdk:"team_id"`
	MemberID    types.Int64  `tfsdk:"member_id"`
	UserID      types.String `tfsdk:"user_id"`
	InviteEmail types.String `tfsdk:"invite_email"`
	IMRole      types.String `tfsdk:"im_role"`
}

func (m *imTeamMemberModel) fromAPI(teamID int64, in *client.IMTeamMember) {
	m.ID = types.StringValue(idPair(teamID, in.ID))
	m.TeamID = types.Int64Value(teamID)
	m.MemberID = types.Int64Value(in.ID)
	m.UserID = types.StringValue(in.UserID)
	m.IMRole = types.StringValue(in.IMRole)
}

func NewIMTeamMemberResource() resource.Resource { return &imTeamMemberResource{} }

func (r *imTeamMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_team_member"
}

func (r *imTeamMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A member of an Incident Management team. Set `user_id` for an existing login (admin, editor or responder of the organization), " +
			"or `invite_email` to invite by email (an unknown address gets a new account and an invitation mail). " +
			"Import with `<team_id>/<member_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "`<team_id>/<member_id>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
			"member_id": schema.Int64Attribute{Computed: true, Description: "Numeric membership id.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"user_id": schema.StringAttribute{Optional: true, Computed: true, Description: "User id (see data source `uptimeify_user`). Changing it replaces the membership.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured(), stringplanmodifier.UseStateForUnknown()}},
			"invite_email": schema.StringAttribute{Optional: true, Description: "Invite by email instead of `user_id`; sends an invitation mail for a new address. " +
				"Only used on create, not read back.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(3)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"im_role": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("member"),
				Description: "`admin`, `member` or `stakeholder`. Default `member`.", Validators: []validator.String{stringvalidator.OneOf(imRoles...)}},
		},
	}
}

func (r *imTeamMemberResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{resourcevalidator.ExactlyOneOf(path.MatchRoot("user_id"), path.MatchRoot("invite_email"))}
}

func (r *imTeamMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *imTeamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imTeamMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	teamID, role := plan.TeamID.ValueInt64(), plan.IMRole.ValueString()
	var m *client.IMTeamMember
	var err error
	if !plan.InviteEmail.IsNull() {
		m, err = r.client.InviteIMTeamMember(ctx, teamID, plan.InviteEmail.ValueString(), role)
	} else {
		m, err = r.client.AddIMTeamMember(ctx, teamID, plan.UserID.ValueString(), role)
	}
	if err != nil {
		resp.Diagnostics.AddError("Adding IM team member failed", err.Error())
		return
	}
	plan.fromAPI(teamID, m)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imTeamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imTeamMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.client.GetIMTeamMember(ctx, state.TeamID.ValueInt64(), state.MemberID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading IM team member failed", err.Error())
		return
	}
	state.fromAPI(state.TeamID.ValueInt64(), m)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imTeamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state imTeamMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	teamID, memberID := state.TeamID.ValueInt64(), state.MemberID.ValueInt64()
	if !plan.IMRole.Equal(state.IMRole) {
		if err := r.client.UpdateIMTeamMemberRole(ctx, teamID, memberID, plan.IMRole.ValueString()); err != nil {
			resp.Diagnostics.AddError("Updating IM team member failed", err.Error())
			return
		}
	}
	m, err := r.client.GetIMTeamMember(ctx, teamID, memberID)
	if err != nil {
		resp.Diagnostics.AddError("Reading IM team member failed", err.Error())
		return
	}
	plan.fromAPI(teamID, m)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imTeamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imTeamMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIMTeamMember(ctx, state.TeamID.ValueInt64(), state.MemberID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Removing IM team member failed", err.Error())
	}
}

func (r *imTeamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	teamID, memberID, err := parseTwoIDs(req.ID, "<team_id>/<member_id>")
	if err != nil {
		resp.Diagnostics.AddError("Invalid import id", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_id"), teamID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("member_id"), memberID)...)
}
