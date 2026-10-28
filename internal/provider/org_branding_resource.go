package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &brandingResource{}
	_ resource.ResourceWithImportState = &brandingResource{}
	_ resource.ResourceWithModifyPlan  = &brandingResource{}

	// normalizeThemeColorValue of the app: uppercase 6-digit hex or a lowercase palette token.
	brandingColorRe = regexp.MustCompile(`^(#[0-9A-F]{6}|brand|slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)$`)
	brandingURLRe   = regexp.MustCompile(`^https?://\S+$`)
)

// brandingAssets: attribute prefix -> upload kind.
var brandingAssets = []struct{ attr, kind string }{
	{"logo_light", "logoLight"}, {"logo_dark", "logoDark"}, {"favicon_light", "faviconLight"}, {"favicon_dark", "faviconDark"},
}

type brandingResource struct{ client *client.Client }

type brandingModel struct {
	ID                 types.String `tfsdk:"id"`
	ProductName        types.String `tfsdk:"product_name"`
	HideProductName    types.Bool   `tfsdk:"hide_product_name"`
	HideLogos          types.Bool   `tfsdk:"hide_logos"`
	ThemePrimary       types.String `tfsdk:"theme_primary"`
	ThemeSecondary     types.String `tfsdk:"theme_secondary"`
	ImprintURL         types.String `tfsdk:"imprint_url"`
	PrivacyURL         types.String `tfsdk:"privacy_url"`
	TermsURL           types.String `tfsdk:"terms_url"`
	LogoLightFile      types.String `tfsdk:"logo_light_file"`
	LogoLightSHA256    types.String `tfsdk:"logo_light_sha256"`
	LogoDarkFile       types.String `tfsdk:"logo_dark_file"`
	LogoDarkSHA256     types.String `tfsdk:"logo_dark_sha256"`
	FaviconLightFile   types.String `tfsdk:"favicon_light_file"`
	FaviconLightSHA256 types.String `tfsdk:"favicon_light_sha256"`
	FaviconDarkFile    types.String `tfsdk:"favicon_dark_file"`
	FaviconDarkSHA256  types.String `tfsdk:"favicon_dark_sha256"`
}

func (m *brandingModel) fromAPI(b *client.Branding) {
	m.ID = types.StringValue("branding")
	m.ProductName = types.StringPointerValue(b.ProductName)
	m.HideProductName = types.BoolValue(b.HideProductName)
	m.HideLogos = types.BoolValue(b.HideLogos)
	m.ThemePrimary = types.StringPointerValue(b.ThemePrimary)
	m.ThemeSecondary = types.StringPointerValue(b.ThemeSecondary)
	m.ImprintURL = types.StringPointerValue(b.ImprintURL)
	m.PrivacyURL = types.StringPointerValue(b.PrivacyURL)
	m.TermsURL = types.StringPointerValue(b.TermsURL)
}

// patch sends every field (unset = null clears it), so the organization's branding equals the config.
func (m *brandingModel) patch() map[string]any {
	return map[string]any{
		"productName":     m.ProductName.ValueStringPointer(),
		"hideProductName": m.HideProductName.ValueBool(),
		"hideLogos":       m.HideLogos.ValueBool(),
		"themePrimary":    m.ThemePrimary.ValueStringPointer(),
		"themeSecondary":  m.ThemeSecondary.ValueStringPointer(),
		"imprintUrl":      m.ImprintURL.ValueStringPointer(),
		"privacyUrl":      m.PrivacyURL.ValueStringPointer(),
		"termsUrl":        m.TermsURL.ValueStringPointer(),
	}
}

func NewBrandingResource() resource.Resource { return &brandingResource{} }

func (r *brandingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_branding"
}

func (r *brandingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	color := func(what string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Description: what + " color: uppercase hex `#RRGGBB` or a palette name " +
			"(`brand`, `slate`, `gray`, `zinc`, `neutral`, `stone`, `red`, `orange`, `amber`, `yellow`, `lime`, `green`, " +
			"`emerald`, `teal`, `cyan`, `sky`, `blue`, `indigo`, `violet`, `purple`, `fuchsia`, `pink`, `rose`). Unset = the default.",
			Validators: []validator.String{stringvalidator.RegexMatches(brandingColorRe, "must be #RRGGBB in uppercase or a palette name")}}
	}
	legal := func(what string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Description: "Link to your own " + what + ", `http://` or `https://`, up to 500 characters.",
			Validators: []validator.String{stringvalidator.LengthAtMost(500), stringvalidator.RegexMatches(brandingURLRe, "must be an http:// or https:// URL")}}
	}
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"product_name": schema.StringAttribute{Optional: true, Description: "Product name shown instead of Uptimeify, 1 to 80 characters. Unset = Uptimeify.",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 80), statusPageTrimmed}},
		"hide_product_name": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Hide the product name next to the logo. Default `false`."},
		"hide_logos": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Show no logo at all. Default `false`."},
		"theme_primary":   color("Primary"),
		"theme_secondary": color("Secondary"),
		"imprint_url":     legal("imprint"),
		"privacy_url":     legal("privacy policy"),
		"terms_url":       legal("terms"),
	}
	for _, a := range brandingAssets {
		what := "Logo (PNG, JPEG or WebP, at most 2 MiB)"
		if a.kind == "faviconLight" || a.kind == "faviconDark" {
			what = "Favicon (PNG or ICO, at most 512 KiB)"
		}
		attrs[a.attr+"_file"] = schema.StringAttribute{Optional: true,
			Description: what + " for the " + map[bool]string{true: "dark", false: "light"}[a.kind == "logoDark" || a.kind == "faviconDark"] +
				" theme: path of a local file. Uploaded when its content changes (see `" + a.attr + "_sha256`). Removing it " +
				"deletes the uploaded image."}
		attrs[a.attr+"_sha256"] = schema.StringAttribute{Computed: true,
			Description: "SHA-256 of `" + a.attr + "_file` at the last upload, read at plan time."}
	}
	resp.Schema = schema.Schema{
		Description: "White-label branding of the organization (one per organization): product name, colors, legal links, " +
			"logos and favicons. Organization admins only. Destroying it resets name, colors, links and both flags and deletes " +
			"the four images. Import with any id, e.g. `branding`; the image files are not imported.",
		Attributes: attrs,
	}
}

func (r *brandingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// ModifyPlan hashes each configured file, so a changed image plans an upload.
func (r *brandingResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	for _, a := range brandingAssets {
		var file types.String
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(a.attr+"_file"), &file)...)
		sum := types.StringNull()
		switch {
		case file.IsUnknown():
			sum = types.StringUnknown()
		case !file.IsNull():
			data, err := os.ReadFile(file.ValueString())
			if err != nil {
				resp.Diagnostics.AddAttributeError(path.Root(a.attr+"_file"), "Reading branding file failed", err.Error())
				continue
			}
			h := sha256.Sum256(data)
			sum = types.StringValue(hex.EncodeToString(h[:]))
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(a.attr+"_sha256"), sum)...)
	}
}

// apply PATCHes the fields, uploads every file whose hash differs from prior (nil = create) and saves the result.
func (r *brandingResource) apply(ctx context.Context, plan tfsdk.Plan, prior *tfsdk.State, st *tfsdk.State) (diags diag.Diagnostics) {
	var m brandingModel
	if diags.Append(plan.Get(ctx, &m)...); diags.HasError() {
		return
	}
	if err := r.client.UpdateBranding(ctx, m.patch()); err != nil {
		diags.AddError("Saving branding failed", err.Error())
		return
	}
	for _, a := range brandingAssets {
		var file, sum, old types.String
		diags.Append(plan.GetAttribute(ctx, path.Root(a.attr+"_file"), &file)...)
		diags.Append(plan.GetAttribute(ctx, path.Root(a.attr+"_sha256"), &sum)...)
		if prior != nil {
			diags.Append(prior.GetAttribute(ctx, path.Root(a.attr+"_sha256"), &old)...)
		}
		if file.IsNull() {
			if !old.IsNull() { // the file attribute was removed: delete the uploaded image
				if err := r.deleteAsset(ctx, a.kind); err != nil {
					diags.AddAttributeError(path.Root(a.attr+"_file"), "Deleting "+a.attr+" failed", err.Error())
					return
				}
			}
			continue
		}
		if sum.Equal(old) {
			continue
		}
		data, err := os.ReadFile(file.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root(a.attr+"_file"), "Reading branding file failed", err.Error())
			return
		}
		h := sha256.Sum256(data)
		if got := hex.EncodeToString(h[:]); got != sum.ValueString() {
			diags.AddAttributeError(path.Root(a.attr+"_file"), "Branding file changed after plan", "plan again")
			return
		}
		dataURL := "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
		if err := r.client.UploadBrandingAsset(ctx, a.kind, filepath.Base(file.ValueString()), dataURL); err != nil {
			diags.AddAttributeError(path.Root(a.attr+"_file"), "Uploading "+a.attr+" failed", err.Error())
			return
		}
	}
	b, err := r.client.GetBranding(ctx)
	if err != nil {
		diags.AddError("Reading branding failed", err.Error())
		return
	}
	m.fromAPI(b)
	return st.Set(ctx, &m)
}

func (r *brandingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.Append(r.apply(ctx, req.Plan, nil, &resp.State)...)
}

func (r *brandingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.Append(r.apply(ctx, req.Plan, &req.State, &resp.State)...)
}

func (r *brandingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state brandingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBranding(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading branding failed", err.Error())
		return
	}
	state.fromAPI(b) // file and hash attributes stay: the API does not return image content
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// deleteAsset treats 404 (nothing uploaded) as done.
func (r *brandingResource) deleteAsset(ctx context.Context, kind string) error {
	if err := r.client.DeleteBrandingAsset(ctx, kind); err != nil && !client.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *brandingResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	empty := brandingModel{HideProductName: types.BoolValue(false), HideLogos: types.BoolValue(false)}
	if err := r.client.UpdateBranding(ctx, empty.patch()); err != nil {
		resp.Diagnostics.AddError("Resetting branding failed", err.Error())
		return
	}
	for _, a := range brandingAssets {
		if err := r.deleteAsset(ctx, a.kind); err != nil {
			resp.Diagnostics.AddError("Deleting "+a.attr+" failed", err.Error())
		}
	}
}

func (r *brandingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
