package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &packageConfigResource{}
	_ resource.ResourceWithImportState = &packageConfigResource{}
)

// packageConfigResource manages one package of the organization (PATCH/DELETE
// /api/package-configs/{packageType}). It maps the limits only; the check and alert defaults of
// the package stay as the server or the UI set them.
// ponytail: limits only, add the check/alert defaults when someone manages them as code.
type packageConfigResource struct{ client *client.Client }

type packageConfigModel struct {
	ID                 types.String `tfsdk:"id"`
	PackageType        types.String `tfsdk:"package_type"`
	DisplayName        types.String `tfsdk:"display_name"`
	MaxURLs            types.Int64  `tfsdk:"max_urls"`
	MaxStatusPages     types.Int64  `tfsdk:"max_status_pages"`
	AllowSelfService   types.Bool   `tfsdk:"allow_self_service"`
	MaxSelfServiceURLs types.Int64  `tfsdk:"max_self_service_urls"`
	Notes              types.String `tfsdk:"notes"`
}

func (m *packageConfigModel) fromAPI(p *client.PackageConfig) {
	m.ID = types.StringValue(p.PackageType)
	m.PackageType = types.StringValue(p.PackageType)
	m.DisplayName = types.StringPointerValue(p.DisplayName)
	m.MaxURLs = types.Int64PointerValue(p.MaxURLs)
	m.MaxStatusPages = types.Int64PointerValue(p.MaxStatusPages)
	m.AllowSelfService = types.BoolValue(p.AllowSelfService)
	m.MaxSelfServiceURLs = types.Int64Value(p.MaxSelfServiceURLs)
	m.Notes = types.StringPointerValue(p.Notes)
}

// packageConfigPatch: on create (state nil) every known value, on update only the changed ones.
// max_status_pages is sent as null when unset (unlimited).
func packageConfigPatch(plan, state *packageConfigModel) map[string]any {
	patch := map[string]any{}
	if !plan.DisplayName.IsUnknown() && (state == nil || !plan.DisplayName.Equal(state.DisplayName)) && !plan.DisplayName.IsNull() {
		patch["displayName"] = plan.DisplayName.ValueString()
	}
	if !plan.MaxURLs.IsUnknown() && !plan.MaxURLs.IsNull() && (state == nil || !plan.MaxURLs.Equal(state.MaxURLs)) {
		patch["maxUrls"] = plan.MaxURLs.ValueInt64()
	}
	if state == nil || !plan.MaxStatusPages.Equal(state.MaxStatusPages) {
		patch["maxStatusPages"] = plan.MaxStatusPages.ValueInt64Pointer()
	}
	if !plan.AllowSelfService.IsUnknown() && (state == nil || !plan.AllowSelfService.Equal(state.AllowSelfService)) {
		patch["allowSelfService"] = plan.AllowSelfService.ValueBool()
	}
	if !plan.MaxSelfServiceURLs.IsUnknown() && (state == nil || !plan.MaxSelfServiceURLs.Equal(state.MaxSelfServiceURLs)) {
		patch["maxSelfServiceUrls"] = plan.MaxSelfServiceURLs.ValueInt64()
	}
	// The API cannot clear notes (null is ignored), so unset keeps them like display_name.
	if !plan.Notes.IsUnknown() && !plan.Notes.IsNull() && (state == nil || !plan.Notes.Equal(state.Notes)) {
		patch["notes"] = plan.Notes.ValueString()
	}
	return patch
}

func NewPackageConfigResource() resource.Resource { return &packageConfigResource{} }

func (r *packageConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package_config"
}

func (r *packageConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "One package of the organization, addressed by its `package_type` (`PATCH /api/package-configs/{packageType}`). " +
			"Manages the limits only: monitors, status pages and self-service per customer of the package. The check and alert " +
			"defaults of the package stay as they are (the server's defaults for a new package); read them with " +
			"`data.uptimeify_package_configs`. If the package already exists, creating the resource adopts it. Destroy deletes " +
			"the package and fails (`packageInUse`) while customers use it. Import with the package type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "Same as `package_type`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"package_type": schema.StringAttribute{Required: true, Description: "Package key, 1 to 64 characters, no surrounding " +
				"whitespace, e.g. `essential`. Changing it replaces the package.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 64), statusPageTrimmed}},
			"display_name": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Name shown in the UI, 1 to 64 characters. Unset = kept as is.",
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 64), statusPageTrimmed}},
			"max_urls": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Monitors per customer. Unset = kept as is (a new package uses the default of its type).",
				Validators:  []validator.Int64{int64validator.AtLeast(0)}},
			"max_status_pages": schema.Int64Attribute{Optional: true,
				Description: "Status pages per customer of this package. Unset = unlimited. Pages without a customer never count.",
				Validators:  []validator.Int64{int64validator.AtLeast(0)}},
			"allow_self_service": schema.BoolAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Description:   "Customers of the package may create their own (`self_service`) monitors and status pages. Unset = kept as is."},
			"max_self_service_urls": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI,
				Description: "Self-service monitors per customer. Unset = kept as is.",
				Validators:  []validator.Int64{int64validator.AtLeast(0)}},
			"notes": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Internal notes, up to 4096 characters. Unset = kept as is (the API cannot clear them).",
				Validators:    []validator.String{stringvalidator.LengthAtMost(4096)}},
		},
	}
}

func (r *packageConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *packageConfigResource) write(ctx context.Context, plan, state *packageConfigModel) (*packageConfigModel, error) {
	p, err := r.client.UpsertPackageConfig(ctx, plan.PackageType.ValueString(), packageConfigPatch(plan, state))
	if err != nil {
		return nil, err
	}
	plan.fromAPI(p)
	return plan, nil
}

func (r *packageConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan packageConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.write(ctx, &plan, nil)
	if err != nil {
		resp.Diagnostics.AddError("Creating package configuration failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *packageConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state packageConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pcs, err := r.client.ListPackageConfigs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading package configuration failed", err.Error())
		return
	}
	for i := range pcs {
		if pcs[i].PackageType == state.ID.ValueString() {
			state.fromAPI(&pcs[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *packageConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state packageConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.write(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Updating package configuration failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *packageConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state packageConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePackageConfig(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting package configuration failed",
			fmt.Sprintf("%v (a package still used by customers cannot be deleted; move them to another package first)", err))
	}
}

func (r *packageConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("package_type"), req.ID)...)
}
