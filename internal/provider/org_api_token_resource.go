package provider

import (
	"context"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &apiTokenResource{}
	_ resource.ResourceWithImportState = &apiTokenResource{}
)

// apiTokenResource serves uptimeify_api_token (/api/organization/tokens) and
// uptimeify_customer_api_token (/api/customer/tokens). Neither has a PATCH in the API contract,
// so every argument forces a new token.
type apiTokenResource struct {
	client   *client.Client
	typeName string
	base     string
	customer bool
}

type apiTokenModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	CustomerID    types.Int64  `tfsdk:"customer_id"`
	ExpiresInDays types.Int64  `tfsdk:"expires_in_days"`
	ExpiresAt     types.String `tfsdk:"expires_at"`
	CreatedAt     types.String `tfsdk:"created_at"`
	LastUsedAt    types.String `tfsdk:"last_used_at"`
	TokenHint     types.String `tfsdk:"token_hint"`
	Token         types.String `tfsdk:"token"`
}

func (m *apiTokenModel) fromAPI(t *client.APIToken) {
	m.ID = types.StringValue(strconv.FormatInt(t.ID, 10))
	m.Name = types.StringValue(t.Name)
	m.CustomerID = types.Int64PointerValue(t.CustomerID)
	m.ExpiresAt = types.StringPointerValue(t.ExpiresAt)
	m.CreatedAt = types.StringValue(t.CreatedAt)
	m.LastUsedAt = types.StringPointerValue(t.LastUsedAt)
	m.TokenHint = types.StringValue(t.TokenHint)
}

func NewAPITokenResource() resource.Resource {
	return &apiTokenResource{typeName: "_api_token", base: "/api/organization/tokens"}
}

func NewCustomerAPITokenResource() resource.Resource {
	return &apiTokenResource{typeName: "_customer_api_token", base: "/api/customer/tokens", customer: true}
}

func (r *apiTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.typeName
}

func (r *apiTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replaceInt := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	desc := "An organization API token (`wsm_...`, full organization access unless `customer_id` is set). " +
		"Only a signed-in organization admin can create, list or revoke these: the API answers API-token requests " +
		"on `/api/organization/tokens` with 401, so with the provider's `wsm_` token every operation fails with a clear " +
		"error until the API accepts tokens there. Use `uptimeify_customer_api_token` for customer-scoped tokens."
	customer := schema.Int64Attribute{Optional: true, PlanModifiers: replaceInt, Validators: []validator.Int64{int64validator.AtLeast(1)},
		Description: "Restrict the token to this customer. Unset = whole organization."}
	if r.customer {
		desc = "A customer-scoped API token (`wsm_...`, limited to one customer). A token created by Terraform with an " +
			"API token runs with the role `readonly`, because its issuer is a token, not a person."
		customer = schema.Int64Attribute{Required: true, PlanModifiers: replaceInt, Validators: []validator.Int64{int64validator.AtLeast(1)},
			Description: "The customer the token is limited to. Required: the API cannot revoke an organization-wide token created here."}
	}
	resp.Schema = schema.Schema{
		Description: desc + " The token value is returned once, on create, and kept in the state as a sensitive value; it cannot " +
			"be read again, so an imported token has no `token`. Every change creates a new token. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{stringvalidator.LengthBetween(1, 255)}},
			"customer_id": customer,
			"expires_in_days": schema.Int64Attribute{Optional: true, PlanModifiers: replaceInt,
				Validators:  []validator.Int64{int64validator.Between(1, 365)},
				Description: "Lifetime in days from creation. Unset = never expires. Only sent on create (not readable; see `expires_at`)."},
			"expires_at":   schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"created_at":   schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"last_used_at": schema.StringAttribute{Computed: true},
			"token_hint":   schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Short recognition hint shown in the dashboard."},
			"token": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keep,
				Description: "The token itself. Only known after create; empty after import."},
		},
	}
}

func (r *apiTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// explain turns the two known refusals into actionable messages.
func (r *apiTokenResource) explain(err error) string {
	apiErr, ok := err.(*client.APIError)
	if !ok {
		return err.Error()
	}
	switch {
	case apiErr.Code == "mfa_step_up_required":
		return err.Error() + "\nThe signed-in user has two-step verification and must re-confirm it before managing tokens; " +
			"this cannot be done through the API."
	case !r.customer && apiErr.StatusCode == http.StatusUnauthorized:
		return err.Error() + "\nOrganization tokens can only be managed from a signed-in admin session (dashboard). The API " +
			"refuses `wsm_` API tokens on /api/organization/tokens; use uptimeify_customer_api_token for customer tokens."
	}
	return err.Error()
}

func (r *apiTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateAPIToken(ctx, r.base, client.APITokenCreate{
		Name:          plan.Name.ValueString(),
		ExpiresInDays: plan.ExpiresInDays.ValueInt64Pointer(),
		CustomerID:    plan.CustomerID.ValueInt64Pointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating API token failed", r.explain(err))
		return
	}
	plan.Token = types.StringValue(created.Token)
	t, err := r.client.GetAPIToken(ctx, r.base, created.ID)
	if err != nil {
		resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(created.ID, 10))
		resp.Diagnostics.AddError("Reading created API token failed", r.explain(err))
		return
	}
	plan.fromAPI(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid token id", err.Error())
		return
	}
	t, err := r.client.GetAPIToken(ctx, r.base, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading API token failed", r.explain(err))
		return
	}
	state.fromAPI(t)
	if state.Token.IsUnknown() {
		state.Token = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached: every argument forces replacement.
func (r *apiTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan apiTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	id, _ := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err := r.client.DeleteAPIToken(ctx, r.base, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Revoking API token failed", r.explain(err))
	}
}

func (r *apiTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
