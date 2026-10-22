package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// UserRoles is ALLOWED_USER_ROLES (server/lib/users/user-write-schemas.ts).
var UserRoles = []string{"admin", "editor", "readonly", "responder"}

// UserLanguages is MAIL_LOCALES (@uptimeify/mail-core).
var UserLanguages = []string{"de", "en", "da"}

var (
	_ resource.ResourceWithConfigure      = &orgUserResource{}
	_ resource.ResourceWithImportState    = &orgUserResource{}
	_ resource.ResourceWithValidateConfig = &orgUserResource{}
)

type orgUserResource struct{ client *client.Client }

type orgUserModel struct {
	ID          types.String `tfsdk:"id"`
	Email       types.String `tfsdk:"email"`
	FirstName   types.String `tfsdk:"first_name"`
	LastName    types.String `tfsdk:"last_name"`
	Name        types.String `tfsdk:"name"`
	Role        types.String `tfsdk:"role"`
	IsActive    types.Bool   `tfsdk:"is_active"`
	CustomerIDs types.Set    `tfsdk:"customer_ids"`
	Language    types.String `tfsdk:"language"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

// emptyAsNull: a cleared first/last name comes back as "".
func emptyAsNull(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func (m *orgUserModel) fromAPI(u *client.UserDetail) {
	m.ID = types.StringValue(u.ID)
	m.Email = types.StringValue(u.Email)
	m.FirstName = emptyAsNull(u.FirstName)
	m.LastName = emptyAsNull(u.LastName)
	m.Name = types.StringPointerValue(u.Name)
	m.Role = types.StringValue(u.Role)
	m.IsActive = types.BoolValue(u.IsActive)
	ids := make([]attr.Value, 0, len(u.CustomerAccess))
	for _, a := range u.CustomerAccess {
		ids = append(ids, types.Int64Value(a.CustomerID))
	}
	m.CustomerIDs = types.SetValueMust(types.Int64Type, ids)
	m.CreatedAt = types.StringValue(u.CreatedAt)
	m.Language = types.StringPointerValue(u.Language)
}

func (m *orgUserModel) customerIDs(ctx context.Context) []int64 {
	var ids []int64
	m.CustomerIDs.ElementsAs(ctx, &ids, false)
	return ids
}

func NewOrgUserResource() resource.Resource { return &orgUserResource{} }

func (r *orgUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *orgUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A login of the organization. Creating it sends an invitation mail; the person sets their own " +
			"password (Terraform never sets one). Destroying it deletes the login. Needs an organization admin token. " +
			"Import with the user id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "User id, as `user_id` in IM resources."},
			"email": schema.StringAttribute{Required: true, PlanModifiers: replace,
				Description: "Login address. Changing it invites a new login (replacement)."},
			"first_name": schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 100)}},
			"last_name":  schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 100)}},
			"name":       schema.StringAttribute{Computed: true, Description: "Display name the server derives from first and last name."},
			"role": schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf(UserRoles...)},
				Description: "`admin`, `editor`, `readonly` or `responder`."},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "`false` deactivates the login without deleting it."},
			"customer_ids": schema.SetAttribute{ElementType: types.Int64Type, Optional: true, Computed: true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.Int64Type, nil)),
				Description: "Customers a `readonly` login is limited to (customer user). Empty = all customers. Only allowed with `role = \"readonly\"`."},
			"language": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplaceIfConfigured()},
				Validators:    []validator.String{stringvalidator.OneOf(UserLanguages...)},
				Description: "Language of the login (dashboard, mails); null = organization language. Only sent on create: the API " +
					"cannot change it later, the person changes it in their profile. Unset = read from the API only; set, a " +
					"different value read back (also one the person chose) replaces the login."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *orgUserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg orgUserModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Role.IsUnknown() || cfg.Role.IsNull() || cfg.CustomerIDs.IsUnknown() || cfg.CustomerIDs.IsNull() {
		return
	}
	if cfg.Role.ValueString() != "readonly" && len(cfg.CustomerIDs.Elements()) > 0 {
		resp.Diagnostics.AddAttributeError(path.Root("customer_ids"), "customer_ids needs role readonly",
			"Only readonly logins can be limited to customers (API code customerIdsReadonlyOnly).")
	}
}

func (r *orgUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *orgUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.CreateUser(ctx, client.UserCreate{
		Email:       plan.Email.ValueString(),
		FirstName:   plan.FirstName.ValueStringPointer(),
		LastName:    plan.LastName.ValueStringPointer(),
		Role:        plan.Role.ValueString(),
		CustomerIDs: plan.customerIDs(ctx),
		Language:    plan.Language.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Inviting user failed", err.Error())
		return
	}
	var u *client.UserDetail
	if !plan.IsActive.ValueBool() {
		u, err = r.client.UpdateUser(ctx, id, map[string]any{"isActive": false})
	} else {
		u, err = r.client.GetUser(ctx, id)
	}
	if err != nil {
		// The invitation exists: keep the id so the next apply converges instead of re-inviting.
		resp.State.SetAttribute(ctx, path.Root("id"), id)
		resp.Diagnostics.AddError("Reading invited user failed", err.Error())
		return
	}
	plan.fromAPI(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *orgUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	u, err := r.client.GetUser(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading user failed", err.Error())
		return
	}
	state.fromAPI(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *orgUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	// The server rebuilds `name` from the names in the body only, so both go together.
	if !plan.FirstName.Equal(state.FirstName) || !plan.LastName.Equal(state.LastName) {
		patch["firstName"] = plan.FirstName.ValueString()
		patch["lastName"] = plan.LastName.ValueString()
	}
	if !plan.Role.Equal(state.Role) {
		patch["role"] = plan.Role.ValueString()
	}
	if !plan.IsActive.Equal(state.IsActive) {
		patch["isActive"] = plan.IsActive.ValueBool()
	}
	if !plan.CustomerIDs.Equal(state.CustomerIDs) && plan.Role.ValueString() == "readonly" {
		patch["customerIds"] = append([]int64{}, plan.customerIDs(ctx)...)
	}
	u, err := r.client.UpdateUser(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating user failed", err.Error())
		return
	}
	plan.fromAPI(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *orgUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteUser(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting user failed", err.Error())
	}
}

func (r *orgUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
