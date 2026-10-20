package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ resource.ResourceWithConfigure      = &notificationChannelResource{}
	_ resource.ResourceWithImportState    = &notificationChannelResource{}
	_ resource.ResourceWithValidateConfig = &notificationChannelResource{}
)

type notificationChannelResource struct{ client *client.Client }

// notificationChannelModel is shared by the uptimeify_notification_channel resource and data source.
type notificationChannelModel struct {
	ID                  types.String `tfsdk:"id"`
	OrganizationID      types.Int64  `tfsdk:"organization_id"`
	CustomerID          types.Int64  `tfsdk:"customer_id"`
	Type                types.String `tfsdk:"type"`
	Category            types.String `tfsdk:"category"`
	Name                types.String `tfsdk:"name"`
	Config              types.String `tfsdk:"config"`
	SecretsSet          types.Map    `tfsdk:"secrets_set"`
	DelaySeconds        types.Int64  `tfsdk:"delay_seconds"`
	Conditions          types.String `tfsdk:"conditions"`
	AllowedPackageTypes types.Set    `tfsdk:"allowed_package_types"`
	IsActive            types.Bool   `tfsdk:"is_active"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

type notificationChannelResourceModel struct {
	notificationChannelModel
	SecretConfigWO        types.String `tfsdk:"secret_config_wo"`
	SecretConfigWOVersion types.Int64  `tfsdk:"secret_config_wo_version"`
}

// secretKeys of a channel type: the config keys and the has-flags the API answers instead.
func secretKeys(typ string) (keys, flags map[string]bool) {
	keys, flags = map[string]bool{}, map[string]bool{}
	for _, s := range client.ChannelSecrets[typ] {
		keys[s.Key], flags[s.HasKey] = true, true
	}
	return keys, flags
}

func (m *notificationChannelModel) fromAPI(ctx context.Context, ch *client.NotificationChannel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(strconv.FormatInt(ch.ID, 10))
	m.OrganizationID = types.Int64Value(ch.OrganizationID)
	m.CustomerID = types.Int64PointerValue(ch.CustomerID)
	m.Type = types.StringValue(ch.Type)
	m.Category = types.StringValue(ch.Category)
	m.Name = types.StringValue(ch.Name)
	keys, flags := secretKeys(ch.Type)
	cfg, set := map[string]any{}, map[string]bool{}
	for k, v := range ch.Config {
		switch {
		case flags[k]:
			b, _ := v.(bool)
			set[strings.TrimPrefix(k, "has")] = b
		case keys[k] || v == nil: // redacted secret, or a key cleared with null
		default:
			cfg[k] = v
		}
	}
	m.Config = jsonKeep(m.Config, cfg)
	var d diag.Diagnostics
	m.SecretsSet, d = types.MapValueFrom(ctx, types.BoolType, set)
	diags.Append(d...)
	m.DelaySeconds = types.Int64Value(0)
	if ch.DelaySeconds != nil {
		m.DelaySeconds = types.Int64Value(*ch.DelaySeconds)
	}
	var cond map[string]any
	if len(ch.Conditions) > 0 && json.Unmarshal(ch.Conditions, &cond) == nil && cond != nil {
		m.Conditions = jsonKeep(m.Conditions, cond)
	} else {
		m.Conditions = types.StringNull()
	}
	if ch.AllowedPackageTypes == nil {
		m.AllowedPackageTypes = types.SetNull(types.StringType)
	} else {
		m.AllowedPackageTypes, d = types.SetValueFrom(ctx, types.StringType, ch.AllowedPackageTypes)
		diags.Append(d...)
	}
	m.IsActive = types.BoolValue(ch.IsActive)
	m.CreatedAt = types.StringValue(ch.CreatedAt)
	return diags
}

func NewNotificationChannelResource() resource.Resource { return &notificationChannelResource{} }

func (r *notificationChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_channel"
}

func secretKeyTable() string {
	types := make([]string, 0, len(client.ChannelSecrets))
	for t := range client.ChannelSecrets {
		types = append(types, t)
	}
	sort.Strings(types)
	var b strings.Builder
	for _, t := range types {
		var ks []string
		for _, s := range client.ChannelSecrets[t] {
			ks = append(ks, "`"+s.Key+"`")
		}
		fmt.Fprintf(&b, "`%s`: %s; ", t, strings.Join(ks, ", "))
	}
	return strings.TrimSuffix(b.String(), "; ")
}

func (r *notificationChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A notification channel of the organization, or of one customer with `customer_id`. Secrets live in " +
			"`secret_config_wo` and never enter the state. An active channel to the same recipient cannot exist twice " +
			"(409). Website overrides are not managed here. Import with the numeric id.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric channel id."},
			"organization_id": schema.Int64Attribute{Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"customer_id": schema.Int64Attribute{Optional: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Description:   "Channel of this customer. Unset = organization channel."},
			"type": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description: "Channel type: `" + strings.Join(client.NotificationChannelTypes, "`, `") + "`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.NotificationChannelTypes...)}},
			"category": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
				Description:   "`direct` (default) or `integration`.", Validators: []validator.String{stringvalidator.OneOf("direct", "integration")}},
			"name": schema.StringAttribute{Required: true, Validators: []validator.String{trimmedText, stringvalidator.LengthAtMost(100)}},
			"config": schema.StringAttribute{Required: true, Validators: []validator.String{jsonObject{}},
				Description: "Provider settings without secrets, as a JSON object (`jsonencode({ email = \"ops@example.com\" })`). " +
					"Keys removed here are cleared."},
			"secret_config_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true, Validators: []validator.String{jsonObject{}},
				Description: "Write-only secret settings as a JSON object, stored encrypted. Sent with create and whenever " +
					"`secret_config_wo_version` changes; keys left out keep their stored value. A secret cannot be removed " +
					"(the API keeps the old value), replace the channel instead. Secret keys per type: " + secretKeyTable() + "."},
			"secret_config_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `secret_config_wo`."},
			"secrets_set": schema.MapAttribute{Computed: true, ElementType: types.BoolType,
				Description: "Which secrets are stored, e.g. `{ Secret = true }` (the API's `has<Secret>` flags)."},
			"delay_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Description: "Wait before sending, 0 to 86400 seconds.", Validators: []validator.Int64{int64validator.Between(0, 86400)}},
			"conditions": schema.StringAttribute{Optional: true, Validators: []validator.String{jsonObject{}},
				Description: "Delivery conditions as a JSON object. Unset = always."},
			"allowed_package_types": schema.SetAttribute{Optional: true, ElementType: types.StringType,
				Description: "Only customers with these package types get this channel. Unset = all packages.",
				Validators:  []validator.Set{setvalidator.SizeAtMost(50), setvalidator.ValueStringsAre(trimmedText, stringvalidator.LengthAtMost(64))}},
			"is_active":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

// ValidateConfig keeps secrets out of `config` (they would land in the state and diff forever).
func (r *notificationChannelResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg notificationChannelResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Type.IsUnknown() || cfg.Type.IsNull() {
		return
	}
	keys, _ := secretKeys(cfg.Type.ValueString())
	if s := cfg.Config.ValueString(); !cfg.Config.IsUnknown() && s != "" {
		if m, err := parseJSONObject(s); err == nil {
			for k := range m {
				if keys[k] {
					resp.Diagnostics.AddAttributeError(path.Root("config"), "Secret in config",
						fmt.Sprintf("%q is a secret of %s channels; set it in secret_config_wo.", k, cfg.Type.ValueString()))
				}
			}
		}
	}
	if !cfg.SecretConfigWO.IsNull() && !cfg.SecretConfigWO.IsUnknown() {
		if m, err := parseJSONObject(cfg.SecretConfigWO.ValueString()); err == nil {
			for k := range m {
				if !keys[k] {
					resp.Diagnostics.AddAttributeError(path.Root("secret_config_wo"), "Not a secret",
						fmt.Sprintf("%q is not a secret of %s channels; set it in config.", k, cfg.Type.ValueString()))
				}
			}
		}
	}
}

func (r *notificationChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func packageTypes(ctx context.Context, s types.Set) (any, diag.Diagnostics) {
	if s.IsNull() {
		return nil, nil
	}
	var out []string
	d := s.ElementsAs(ctx, &out, false)
	return out, d
}

func (r *notificationChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg notificationChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	config, _ := parseJSONObject(plan.Config.ValueString())
	if !cfg.SecretConfigWO.IsNull() {
		secrets, _ := parseJSONObject(cfg.SecretConfigWO.ValueString())
		for k, v := range secrets {
			config[k] = v
		}
	}
	body := map[string]any{
		"type": plan.Type.ValueString(), "name": plan.Name.ValueString(), "config": config,
		"delaySeconds": plan.DelaySeconds.ValueInt64(), "isActive": plan.IsActive.ValueBool(),
	}
	if !plan.CustomerID.IsNull() {
		body["customerId"] = plan.CustomerID.ValueInt64()
	}
	if !plan.Category.IsUnknown() {
		body["category"] = plan.Category.ValueString()
	}
	if !plan.Conditions.IsNull() {
		body["conditions"], _ = parseJSONObject(plan.Conditions.ValueString())
	}
	ch, err := r.client.CreateNotificationChannel(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating notification channel failed", err.Error())
		return
	}
	if !plan.AllowedPackageTypes.IsNull() { // not part of the create schema
		pt, d := packageTypes(ctx, plan.AllowedPackageTypes)
		resp.Diagnostics.Append(d...)
		updated, err := r.client.UpdateNotificationChannel(ctx, strconv.FormatInt(ch.ID, 10), map[string]any{"allowedPackageTypes": pt})
		if err != nil {
			resp.Diagnostics.AddError("Setting allowed package types failed", err.Error())
		} else {
			ch = updated
		}
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, ch)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ch, err := r.client.GetNotificationChannel(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading notification channel failed", err.Error())
		return
	}
	resp.Diagnostics.Append(state.fromAPI(ctx, ch)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *notificationChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg notificationChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.DelaySeconds.Equal(state.DelaySeconds) {
		patch["delaySeconds"] = plan.DelaySeconds.ValueInt64()
	}
	if !plan.IsActive.Equal(state.IsActive) {
		patch["isActive"] = plan.IsActive.ValueBool()
	}
	if !plan.Conditions.Equal(state.Conditions) {
		patch["conditions"] = nil
		if !plan.Conditions.IsNull() {
			patch["conditions"], _ = parseJSONObject(plan.Conditions.ValueString())
		}
	}
	if !plan.AllowedPackageTypes.Equal(state.AllowedPackageTypes) {
		pt, d := packageTypes(ctx, plan.AllowedPackageTypes)
		resp.Diagnostics.Append(d...)
		patch["allowedPackageTypes"] = pt
	}
	resendSecrets := !cfg.SecretConfigWO.IsNull() && !plan.SecretConfigWOVersion.Equal(state.SecretConfigWOVersion)
	if resendSecrets || !plan.Config.Equal(state.Config) {
		// The API merges onto the stored config: a removed key must be sent as null.
		config, _ := parseJSONObject(plan.Config.ValueString())
		old, _ := parseJSONObject(state.Config.ValueString())
		for k := range old {
			if _, ok := config[k]; !ok {
				config[k] = nil
			}
		}
		if resendSecrets {
			secrets, _ := parseJSONObject(cfg.SecretConfigWO.ValueString())
			for k, v := range secrets {
				config[k] = v
			}
		}
		patch["config"] = config
	}
	ch, err := r.client.UpdateNotificationChannel(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating notification channel failed", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.fromAPI(ctx, ch)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.ValueString() == "" {
		return
	}
	if err := r.client.DeleteNotificationChannel(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting notification channel failed", err.Error())
	}
}

func (r *notificationChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
