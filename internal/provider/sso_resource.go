package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &ssoOIDCResource{}
	_ resource.ResourceWithImportState = &ssoOIDCResource{}
	_ resource.ResourceWithModifyPlan  = &ssoOIDCResource{}
	_ resource.ResourceWithConfigure   = &ssoDomainVerificationResource{}
	_ resource.ResourceWithImportState = &ssoDomainVerificationResource{}
)

// The organization has at most one SSO configuration, so both resources use the fixed id "sso".
const ssoID = "sso"

// ---- uptimeify_sso_oidc -------------------------------------------------------------------------

type ssoOIDCResource struct{ client *client.Client }

type ssoOIDCModel struct {
	ID                    types.String `tfsdk:"id"`
	Issuer                types.String `tfsdk:"issuer"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecretWO        types.String `tfsdk:"client_secret_wo"`
	ClientSecretWOVersion types.Int64  `tfsdk:"client_secret_wo_version"`
	Domain                types.String `tfsdk:"domain"`
	DefaultRole           types.String `tfsdk:"default_role"`
	EnforceSSO            types.Bool   `tfsdk:"enforce_sso"`
	RedirectURI           types.String `tfsdk:"redirect_uri"`
	Scopes                types.List   `tfsdk:"scopes"`
	DomainVerified        types.Bool   `tfsdk:"domain_verified"`
	CanEnforce            types.Bool   `tfsdk:"can_enforce"`
	VerificationRecord    types.Object `tfsdk:"verification_record"`
}

func (m *ssoOIDCModel) fromAPI(s *client.SSO) {
	m.ID = types.StringValue(ssoID)
	m.Issuer = types.StringValue(s.Issuer)
	m.ClientID = types.StringValue(s.ClientID)
	m.Domain = types.StringValue(s.Domain)
	m.DefaultRole = types.StringValue(s.DefaultRole)
	m.EnforceSSO = types.BoolValue(s.EnforceSSO)
	m.RedirectURI = types.StringValue(s.RedirectURI)
	m.Scopes = stringList(s.Scopes)
	m.DomainVerified = types.BoolValue(s.DomainVerified)
	m.CanEnforce = types.BoolValue(s.CanEnforce)
	m.VerificationRecord = types.ObjectValueMust(verificationRecordAttrTypes, map[string]attr.Value{
		"name": types.StringValue(s.TxtRecordName), "value": types.StringValue(s.TxtRecordValue)})
	m.ClientSecretWO = types.StringNull()
}

func stringList(v []string) types.List {
	elems := make([]attr.Value, len(v))
	for i, s := range v {
		elems[i] = types.StringValue(s)
	}
	return types.ListValueMust(types.StringType, elems)
}

func NewSSOOIDCResource() resource.Resource { return &ssoOIDCResource{} }

func (r *ssoOIDCResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_oidc"
}

func (r *ssoOIDCResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "Single sign-on of the organization with an OpenID Connect identity provider (`PUT /api/organization/sso`). " +
			"One per organization; needs an organization admin token. Saving runs the issuer's discovery, so the issuer must be " +
			"reachable over HTTPS from Uptimeify.\n\n" +
			"Register the client at your identity provider with `redirect_uri` and the `scopes` (both known before the first apply, " +
			"see `GET /api/organization/sso`). Prove the domain with the DNS TXT record `verification_record` and " +
			"`uptimeify_sso_domain_verification`. A new `domain` or `issuer` resets the verification, the first login and `enforce_sso`.\n\n" +
			"Destroying it removes SSO. A SAML configuration is not managed here: it reads as gone, and an apply replaces it with OIDC. " +
			"Import with the id `sso`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keepS, Description: "Always `sso`."},
			"issuer": schema.StringAttribute{Required: true,
				Description: "Issuer URL, `https://`, without trailing slash (the discovery document's `issuer` must match).",
				Validators: []validator.String{stringvalidator.LengthBetween(9, 2048),
					stringvalidator.RegexMatches(regexp.MustCompile(`^https://\S*[^/\s]$`), "must start with https:// and not end with /")}},
			"client_id": schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 255)}},
			"client_secret_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
				Description: "Write-only client secret; required for the first save. Sent with create and whenever " +
					"`client_secret_wo_version` changes; never stored in state. Removing it keeps the stored secret."},
			"client_secret_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `client_secret_wo`."},
			"domain": schema.StringAttribute{Required: true,
				Description: "Lowercase email domain of the users who sign in with SSO, e.g. `example.com`. One organization per domain.",
				Validators: []validator.String{stringvalidator.LengthBetween(3, 253),
					stringvalidator.RegexMatches(statusPageHostnameRe, "must be a lowercase domain with at least one dot")}},
			"default_role": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("readonly"),
				Description: "Role of users created by their first SSO login: `readonly`, `editor`, `responder` or `admin`. Defaults to `readonly`.",
				Validators:  []validator.String{stringvalidator.OneOf("readonly", "editor", "responder", "admin")}},
			"enforce_sso": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Only SSO (no password login) for users of `domain`. Needs a verified domain and one completed SSO login " +
					"(`can_enforce`), else the apply fails with 409 `ssoEnforceNotAllowed`. Defaults to `false`."},
			"redirect_uri": schema.StringAttribute{Computed: true, PlanModifiers: keepS,
				Description: "Redirect (callback) URI to register at the identity provider."},
			"scopes": schema.ListAttribute{Computed: true, ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				Description:   "Scopes Uptimeify requests at login (fixed: `openid`, `email`, `profile`)."},
			"domain_verified": schema.BoolAttribute{Computed: true, PlanModifiers: keepB,
				Description: "Whether the TXT record was verified (see `uptimeify_sso_domain_verification`)."},
			"can_enforce": schema.BoolAttribute{Computed: true, PlanModifiers: keepB,
				Description: "Whether `enforce_sso` may be switched on: domain verified and one SSO login done."},
			"verification_record": schema.SingleNestedAttribute{Computed: true,
				PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Description:   "DNS TXT record that proves control of `domain`; publish it with your DNS provider.",
				Attributes: map[string]schema.Attribute{
					"name":  schema.StringAttribute{Computed: true, Description: "Name of the TXT record, `_uptimeify-sso.<domain>`."},
					"value": schema.StringAttribute{Computed: true, Description: "Value of the TXT record."},
				}},
		},
	}
}

func (r *ssoOIDCResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// ModifyPlan: a new domain or issuer resets the verification server-side and mints a new TXT value.
func (r *ssoOIDCResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state ssoOIDCModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || (plan.Domain.Equal(state.Domain) && plan.Issuer.Equal(state.Issuer)) {
		return
	}
	plan.DomainVerified = types.BoolUnknown()
	plan.CanEnforce = types.BoolUnknown()
	plan.VerificationRecord = types.ObjectUnknown(verificationRecordAttrTypes)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func ssoBody(plan *ssoOIDCModel, secret types.String) map[string]any {
	body := map[string]any{
		"type":        "oidc",
		"issuer":      plan.Issuer.ValueString(),
		"clientId":    plan.ClientID.ValueString(),
		"domain":      plan.Domain.ValueString(),
		"defaultRole": plan.DefaultRole.ValueString(),
		"enforceSso":  plan.EnforceSSO.ValueBool(),
	}
	if !secret.IsNull() && !secret.IsUnknown() {
		body["clientSecret"] = secret.ValueString()
	}
	return body
}

func (r *ssoOIDCResource) put(ctx context.Context, plan *ssoOIDCModel, secret types.String) error {
	s, err := r.client.PutSSO(ctx, ssoBody(plan, secret))
	if err != nil {
		return err
	}
	plan.fromAPI(s)
	return nil
}

func (r *ssoOIDCResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg ssoOIDCModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan, cfg.ClientSecretWO); err != nil {
		resp.Diagnostics.AddError("Saving SSO configuration failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoOIDCResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssoOIDCModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetSSO(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading SSO configuration failed", err.Error())
		return
	}
	if !s.Configured || s.Type != "oidc" {
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ssoOIDCResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg ssoOIDCModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	secret := types.StringNull() // without clientSecret the server keeps the stored one
	if !plan.ClientSecretWOVersion.Equal(state.ClientSecretWOVersion) {
		secret = cfg.ClientSecretWO
	}
	if err := r.put(ctx, &plan, secret); err != nil {
		resp.Diagnostics.AddError("Saving SSO configuration failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoOIDCResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DeleteSSO(ctx); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Removing SSO configuration failed", err.Error())
	}
}

func (r *ssoOIDCResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != ssoID {
		resp.Diagnostics.AddError("Invalid import id", "expected `sso`, got "+fmt.Sprintf("%q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ssoID)...)
}

// ---- uptimeify_sso_domain_verification ----------------------------------------------------------

type ssoDomainVerificationResource struct{ client *client.Client }

type ssoDomainVerificationModel struct {
	ID     types.String `tfsdk:"id"`
	Domain types.String `tfsdk:"domain"`
}

func NewSSODomainVerificationResource() resource.Resource { return &ssoDomainVerificationResource{} }

func (r *ssoDomainVerificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_domain_verification"
}

func (r *ssoDomainVerificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Verifies the SSO domain's DNS TXT record once (`POST /api/organization/sso/verify-domain`). Create fails " +
			"with the expected and the found TXT values when the record is not visible yet; publish " +
			"`uptimeify_sso_oidc.verification_record` and apply again. There is no waiting or retrying. Needs an organization admin token.\n\n" +
			"Set `domain = uptimeify_sso_oidc.<name>.domain`, so a new domain verifies again in the same apply. A new `issuer` " +
			"also resets the verification: the next plan shows this resource again. Destroying it changes nothing in Uptimeify. " +
			"Import with the id `sso`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "Always `sso`."},
			"domain": schema.StringAttribute{Required: true, Description: "The SSO domain; changing it verifies again.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		},
	}
}

func (r *ssoDomainVerificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *ssoDomainVerificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ssoDomainVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetSSO(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading SSO configuration failed", err.Error())
		return
	}
	if !s.Configured || s.Domain != plan.Domain.ValueString() {
		resp.Diagnostics.AddAttributeError(path.Root("domain"), "SSO domain not configured",
			fmt.Sprintf("The organization's SSO domain is %q, not %q. Set domain = uptimeify_sso_oidc.<name>.domain.", s.Domain, plan.Domain.ValueString()))
		return
	}
	if !s.DomainVerified {
		check, err := r.client.VerifySSODomain(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Verifying SSO domain failed", err.Error())
			return
		}
		if !check.Verified {
			found := "none"
			if len(check.Found) > 0 {
				found = strings.Join(check.Found, ", ")
			}
			resp.Diagnostics.AddError("SSO domain not verified",
				fmt.Sprintf("Publish the DNS TXT record %q with the value %q, wait for DNS to propagate, then apply again. Found: %s.",
					s.TxtRecordName, check.Expected, found))
			return
		}
	}
	plan.ID = types.StringValue(ssoID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoDomainVerificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssoDomainVerificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetSSO(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading SSO configuration failed", err.Error())
		return
	}
	// Gone, unverified (new domain or issuer) or another domain: the next apply verifies again.
	if !s.Configured || !s.DomainVerified || (!state.Domain.IsNull() && s.Domain != state.Domain.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(ssoID)
	state.Domain = types.StringValue(s.Domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called with a change: domain requires replacement.
func (r *ssoDomainVerificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ssoDomainVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the verification; the API has no way to undo it.
func (r *ssoDomainVerificationResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *ssoDomainVerificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != ssoID {
		resp.Diagnostics.AddError("Invalid import id", "expected `sso`, got "+fmt.Sprintf("%q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ssoID)...)
}
