package provider

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure            = &imChannelResource{}
	_ resource.ResourceWithImportState          = &imChannelResource{}
	_ resource.ResourceWithValidateConfig       = &imChannelResource{}
	_ datasource.DataSourceWithConfigure        = &imChannelDataSource{}
	_ datasource.DataSourceWithConfigValidators = &imChannelDataSource{}
)

type imChannelResource struct{ client *client.Client }

// imChannelModel is shared by the uptimeify_im_channel resource and data source.
type imChannelModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.Int64  `tfsdk:"organization_id"`
	TeamID         types.Int64  `tfsdk:"team_id"`
	Type           types.String `tfsdk:"type"`
	Name           types.String `tfsdk:"name"`
	Email          types.String `tfsdk:"email"`
	IsActive       types.Bool   `tfsdk:"is_active"`
	HasWebhookURL  types.Bool   `tfsdk:"has_webhook_url"`
	HeaderNames    types.List   `tfsdk:"header_names"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

// imChannelResourceModel adds the write-only secrets (config only, never in state).
type imChannelResourceModel struct {
	imChannelModel
	WebhookURLWO     types.String `tfsdk:"webhook_url_wo"`
	HeadersWO        types.Map    `tfsdk:"headers_wo"`
	SecretsWOVersion types.Int64  `tfsdk:"secrets_wo_version"`
}

func (m *imChannelModel) fromAPI(c *client.ImChannel) {
	m.ID = types.StringValue(strconv.FormatInt(c.ID, 10))
	m.OrganizationID = types.Int64Value(c.OrganizationID)
	m.TeamID = types.Int64PointerValue(c.TeamID)
	m.Type = types.StringValue(c.Type)
	m.Name = types.StringValue(c.Name)
	m.Email = types.StringPointerValue(c.Config.Email)
	m.IsActive = types.BoolValue(c.IsActive)
	m.HasWebhookURL = types.BoolValue(c.Config.HasWebhookURL)
	names := make([]string, 0, len(c.Config.Headers))
	for k := range c.Config.Headers {
		names = append(names, k)
	}
	slices.Sort(names)
	m.HeaderNames = listOf(types.StringType, names, types.StringValue)
	m.CreatedAt = types.StringValue(c.CreatedAt)
	m.UpdatedAt = types.StringValue(c.UpdatedAt)
}

func NewImChannelResource() resource.Resource { return &imChannelResource{} }

func (r *imChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_channel"
}

func (r *imChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An Incident Management paging channel (Slack incoming webhook, generic webhook or email). Escalation " +
			"tiers and the organization fallback page through it. Incident Management must be active for the organization. " +
			"The webhook URL and header values are secrets: the API never returns them, so they are write-only here. " +
			"Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric channel id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"team_id": schema.Int64Attribute{Optional: true, Description: "Team of the organization; unset = organization-wide channel.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"type": schema.StringAttribute{Required: true, Description: "`webhook`, `slack` or `email`.",
				Validators: []validator.String{stringvalidator.OneOf(client.ImChannelTypes...)}},
			"name": schema.StringAttribute{Required: true, Description: "1 to 120 characters, no surrounding whitespace.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 120)}},
			"email": schema.StringAttribute{Optional: true, Description: "`type = \"email\"`: recipient address (required there)."},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Default `true`. The organization fallback channel cannot be deactivated."},
			"webhook_url_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true,
				Description: "`webhook`/`slack`: public http(s) URL, required on create. Write-only: sent on create, on a " +
					"`type` change and whenever `secrets_wo_version` changes."},
			"headers_wo": schema.MapAttribute{ElementType: types.StringType, Optional: true, WriteOnly: true, Sensitive: true,
				Description: "`webhook`/`slack`: extra request headers (no `Host`). Write-only: sent on create and whenever " +
					"`secrets_wo_version` changes, then replacing the stored set (unset = remove all). The names are readable as `header_names`."},
			"secrets_wo_version": schema.Int64Attribute{Optional: true,
				Description: "Change to resend `webhook_url_wo` and `headers_wo`."},
			"has_webhook_url": schema.BoolAttribute{Computed: true, Description: "Whether a webhook URL is stored."},
			"header_names": schema.ListAttribute{ElementType: types.StringType, Computed: true,
				Description: "Names of the stored headers, sorted (values are never returned)."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

// ValidateConfig: the API keeps unknown config keys of other types unencrypted and returns them,
// so secrets are refused for a type that does not treat them as secrets.
func (r *imChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg imChannelResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}
	if cfg.Type.ValueString() == "email" {
		for name, v := range map[string]bool{"webhook_url_wo": !cfg.WebhookURLWO.IsNull(), "headers_wo": !cfg.HeadersWO.IsNull()} {
			if v {
				resp.Diagnostics.AddAttributeError(path.Root(name), name+" not allowed",
					name+" is only for type webhook or slack; for email the API would store it unencrypted.")
			}
		}
	}
}

func (r *imChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func channelHeaders(ctx context.Context, m types.Map) map[string]string {
	out := map[string]string{}
	if !m.IsNull() && !m.IsUnknown() {
		m.ElementsAs(ctx, &out, false)
	}
	return out
}

func (r *imChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg imChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	config := map[string]any{}
	if !cfg.WebhookURLWO.IsNull() {
		config["webhookUrl"] = cfg.WebhookURLWO.ValueString()
	}
	if !cfg.HeadersWO.IsNull() {
		config["headers"] = channelHeaders(ctx, cfg.HeadersWO)
	}
	if !plan.Email.IsNull() {
		config["email"] = plan.Email.ValueString()
	}
	body := map[string]any{"type": plan.Type.ValueString(), "name": plan.Name.ValueString(),
		"config": config, "isActive": plan.IsActive.ValueBool()}
	if !plan.TeamID.IsNull() {
		body["teamId"] = plan.TeamID.ValueInt64()
	}
	ch, err := r.client.CreateImChannel(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating IM channel failed", err.Error())
		return
	}
	plan.fromAPI(ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ch, err := r.client.GetImChannel(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading IM channel failed", err.Error())
		return
	}
	state.fromAPI(ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// channelPatch builds the PATCH body: changed fields only; secrets when the version or the type changes.
func channelPatch(ctx context.Context, plan, state, cfg *imChannelResourceModel) map[string]any {
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	typeChanged := !plan.Type.Equal(state.Type)
	if typeChanged {
		patch["type"] = plan.Type.ValueString()
	}
	if !plan.TeamID.Equal(state.TeamID) {
		patch["teamId"] = plan.TeamID.ValueInt64Pointer() // nil = organization-wide
	}
	if !plan.IsActive.Equal(state.IsActive) {
		patch["isActive"] = plan.IsActive.ValueBool()
	}
	resend := !plan.SecretsWOVersion.Equal(state.SecretsWOVersion)
	config := map[string]any{}
	if !plan.Email.Equal(state.Email) {
		config["email"] = plan.Email.ValueStringPointer() // nil = remove
	}
	if (resend || typeChanged) && !cfg.WebhookURLWO.IsNull() {
		config["webhookUrl"] = cfg.WebhookURLWO.ValueString()
	}
	if resend && plan.Type.ValueString() != "email" {
		config["headers"] = channelHeaders(ctx, cfg.HeadersWO) // replaces the stored set
	}
	if len(config) > 0 {
		patch["config"] = config
	}
	return patch
}

func (r *imChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg imChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	var ch *client.ImChannel
	var err error
	if patch := channelPatch(ctx, &plan, &state, &cfg); len(patch) > 0 {
		ch, err = r.client.UpdateImChannel(ctx, id, patch)
	} else {
		ch, err = r.client.GetImChannel(ctx, id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating IM channel failed", err.Error())
		return
	}
	plan.fromAPI(ch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state imChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteImChannel(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting IM channel failed",
			err.Error()+"\nA channel that is the organization fallback (uptimeify_im_org_settings.fallback_channel_id) cannot be deleted.")
	}
}

func (r *imChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// data source

type imChannelDataSource struct{ client *client.Client }

func NewImChannelDataSource() datasource.DataSource { return &imChannelDataSource{} }

func (d *imChannelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_channel"
}

func (d *imChannelDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&imChannelResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema, "secrets_wo_version")
	attrs["id"] = dschema.StringAttribute{Optional: true, Computed: true, Description: "Numeric channel id."}
	attrs["name"] = dschema.StringAttribute{Optional: true, Computed: true}
	resp.Schema = dschema.Schema{
		Description: "Look up one Incident Management paging channel by `id` or by `name` (exactly one). " +
			"Fails when no or several channels match the name. Secrets are never returned.",
		Attributes: attrs,
	}
}

func (d *imChannelDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *imChannelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *imChannelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.ImChannel
	if !cfg.ID.IsNull() {
		ch, err := d.client.GetImChannel(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading IM channel failed", err.Error())
			return
		}
		found = ch
	} else {
		all, err := d.client.ListImChannels(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing IM channels failed", err.Error())
			return
		}
		all = slices.DeleteFunc(all, func(c client.ImChannel) bool { return c.Name != cfg.Name.ValueString() })
		if len(all) != 1 {
			resp.Diagnostics.AddError("IM channel lookup by name failed",
				fmt.Sprintf("%d channels are named %q, expected exactly 1. Use id instead.", len(all), cfg.Name.ValueString()))
			return
		}
		found = &all[0]
	}
	cfg.fromAPI(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
