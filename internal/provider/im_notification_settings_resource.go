package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Personal notification chains (app migration 0325): one user's settings set by an org admin, and
// the organization's default chain for users whose settings are created from then on.

var (
	_ resource.ResourceWithConfigure   = &userNotificationSettingsResource{}
	_ resource.ResourceWithImportState = &userNotificationSettingsResource{}
	_ resource.ResourceWithConfigure   = &notificationTemplateResource{}
	_ resource.ResourceWithImportState = &notificationTemplateResource{}

	phoneRegexp           = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
	notificationChannels  = []string{"push", "sms", "voice", "email"}
	notificationUrgencies = []string{"high", "low"}
	notificationStepType  = types.ObjectType{AttrTypes: map[string]attr.Type{
		"urgency": types.StringType, "delay_minutes": types.Int64Type, "channel": types.StringType,
	}}
)

type notificationStepModel struct {
	Urgency      types.String `tfsdk:"urgency"`
	DelayMinutes types.Int64  `tfsdk:"delay_minutes"`
	Channel      types.String `tfsdk:"channel"`
}

func notificationRulesAttribute(desc string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Required:    true,
		Description: desc,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"urgency": schema.StringAttribute{Required: true, Description: "`high` or `low`.",
				Validators: []validator.String{stringvalidator.OneOf(notificationUrgencies...)}},
			"delay_minutes": schema.Int64Attribute{Required: true, Description: "Minutes after the page starts, 0 to 1440.",
				Validators: []validator.Int64{int64validator.Between(0, 1440)}},
			"channel": schema.StringAttribute{Required: true,
				Description: "`push`, `sms`, `voice` or `email`. `sms` and `voice` wait until the phone number is verified.",
				Validators:  []validator.String{stringvalidator.OneOf(notificationChannels...)}},
		}},
	}
}

func notificationChannelsAttribute(desc string) schema.MapAttribute {
	return schema.MapAttribute{
		ElementType: types.BoolType, Optional: true, Computed: true, Description: desc,
		Validators: []validator.Map{mapvalidator.KeysAre(stringvalidator.OneOf(notificationChannels...))},
	}
}

func stepsFromSet(ctx context.Context, set types.Set) ([]client.NotificationStep, diag.Diagnostics) {
	var models []notificationStepModel
	diags := set.ElementsAs(ctx, &models, false)
	out := make([]client.NotificationStep, 0, len(models))
	for _, m := range models {
		out = append(out, client.NotificationStep{Urgency: m.Urgency.ValueString(), DelayMinutes: m.DelayMinutes.ValueInt64(), Channel: m.Channel.ValueString()})
	}
	return out, diags
}

func stepsToSet(steps []client.NotificationStep) types.Set {
	elems := make([]attr.Value, 0, len(steps))
	for _, s := range steps {
		elems = append(elems, types.ObjectValueMust(notificationStepType.AttrTypes, map[string]attr.Value{
			"urgency": types.StringValue(s.Urgency), "delay_minutes": types.Int64Value(s.DelayMinutes), "channel": types.StringValue(s.Channel),
		}))
	}
	return types.SetValueMust(notificationStepType, elems)
}

// channelsToMap keeps the four delivery flags (the API map also carries shift-reminder toggles).
// When the configuration named only some channels, only those come back, so the state matches it.
func channelsToMap(api map[string]bool, prior types.Map) types.Map {
	keys := notificationChannels
	if !prior.IsNull() && !prior.IsUnknown() {
		keys = nil
		for k := range prior.Elements() {
			keys = append(keys, k)
		}
	}
	out := map[string]attr.Value{}
	for _, k := range keys {
		out[k] = types.BoolValue(api[k])
	}
	return types.MapValueMust(types.BoolType, out)
}

func channelsBody(ctx context.Context, m types.Map) (map[string]bool, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	out := map[string]bool{}
	return out, m.ElementsAs(ctx, &out, false)
}

// ── uptimeify_user_notification_settings ─────────────────────────────────────

type userNotificationSettingsResource struct{ client *client.Client }

type userNotificationSettingsModel struct {
	ID            types.String `tfsdk:"id"`
	UserID        types.String `tfsdk:"user_id"`
	PhoneNumber   types.String `tfsdk:"phone_number"`
	PhoneVerified types.Bool   `tfsdk:"phone_verified"`
	Channels      types.Map    `tfsdk:"channels"`
	Rules         types.Set    `tfsdk:"rules"`
}

func (m *userNotificationSettingsModel) fromAPI(s *client.UserNotificationSettings) {
	m.ID = types.StringValue(s.UserID)
	m.UserID = types.StringValue(s.UserID)
	m.PhoneNumber = types.StringPointerValue(s.PhoneNumber)
	m.PhoneVerified = types.BoolValue(s.PhoneVerified)
	m.Channels = channelsToMap(s.Channels, m.Channels)
	m.Rules = stepsToSet(s.Rules)
}

func NewUserNotificationSettingsResource() resource.Resource {
	return &userNotificationSettingsResource{}
}

func (r *userNotificationSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_notification_settings"
}

func (r *userNotificationSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Personal Incident Management notification settings of one user of the organization, set by an " +
			"organization admin (`PUT /api/im/users/{userId}/notification-settings`). The last write wins: the user can " +
			"change them afterwards, which shows up as drift. Destroying only removes them from the state, the user keeps " +
			"the chain. Import with the user id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The user id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"user_id": schema.StringAttribute{Required: true,
				Description:   "User of this organization with role admin, editor or responder.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"phone_number": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "E.164 number, e.g. `+491701234567`. Unset = keep the user's number. A new number is unverified " +
					"until the user confirms the SMS code.",
				Validators: []validator.String{stringvalidator.RegexMatches(phoneRegexp, "must be E.164, e.g. +491701234567")}},
			"phone_verified": schema.BoolAttribute{Computed: true,
				Description: "Whether the user confirmed the number. Until then `sms` and `voice` steps are skipped."},
			"channels": notificationChannelsAttribute("Delivery switches `push`, `sms`, `voice`, `email`. Unset = on exactly for the channels the rules use."),
			"rules":    notificationRulesAttribute("The whole notification chain; replaces every existing rule of the user."),
		},
	}
}

func (r *userNotificationSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *userNotificationSettingsResource) put(ctx context.Context, plan *userNotificationSettingsModel) diag.Diagnostics {
	steps, diags := stepsFromSet(ctx, plan.Rules)
	channels, d := channelsBody(ctx, plan.Channels)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	body := map[string]any{"rules": steps}
	if channels != nil {
		body["channels"] = channels
	}
	if v := plan.PhoneNumber; !v.IsNull() && !v.IsUnknown() {
		body["phoneNumber"] = v.ValueString()
	}
	s, err := r.client.PutUserNotificationSettings(ctx, plan.UserID.ValueString(), body)
	if err != nil {
		diags.AddError("Setting notification settings failed", err.Error())
		return diags
	}
	plan.fromAPI(s)
	return diags
}

func (r *userNotificationSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userNotificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.put(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userNotificationSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userNotificationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID := state.UserID.ValueString()
	if userID == "" {
		userID = state.ID.ValueString()
	}
	s, err := r.client.GetUserNotificationSettings(ctx, userID)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading notification settings failed", err.Error())
		return
	}
	state.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userNotificationSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userNotificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.put(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the settings: they belong to the user, who keeps being paged by them.
func (r *userNotificationSettingsResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *userNotificationSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), req.ID)...)
}

// ── uptimeify_notification_template ──────────────────────────────────────────

type notificationTemplateResource struct{ client *client.Client }

type notificationTemplateModel struct {
	ID        types.String `tfsdk:"id"`
	Channels  types.Map    `tfsdk:"channels"`
	Rules     types.Set    `tfsdk:"rules"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (m *notificationTemplateModel) fromAPI(t *client.NotificationTemplate) {
	m.ID = types.StringValue("org")
	m.Channels = channelsToMap(t.Channels, m.Channels)
	m.Rules = stepsToSet(t.Rules)
	m.UpdatedAt = types.StringPointerValue(t.UpdatedAt)
}

func NewNotificationTemplateResource() resource.Resource { return &notificationTemplateResource{} }

func (r *notificationTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_template"
}

func (r *notificationTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The organization's default Incident Management notification chain (one per organization). It is " +
			"copied to a user when their personal settings are first created; existing users keep theirs. Destroying " +
			"switches the template off. Import with any id, e.g. `org`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "Always `org`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"channels":   notificationChannelsAttribute("Delivery switches `push`, `sms`, `voice`, `email` for new users. Unset = on exactly for the channels the rules use."),
			"rules":      notificationRulesAttribute("The default chain."),
			"updated_at": schema.StringAttribute{Computed: true, Description: "Last change of the template."},
		},
	}
}

func (r *notificationTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *notificationTemplateResource) put(ctx context.Context, plan *notificationTemplateModel) diag.Diagnostics {
	steps, diags := stepsFromSet(ctx, plan.Rules)
	channels, d := channelsBody(ctx, plan.Channels)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	body := map[string]any{"rules": steps}
	if channels != nil {
		body["channels"] = channels
	}
	t, err := r.client.PutNotificationTemplate(ctx, body)
	if err != nil {
		diags.AddError("Setting the notification template failed", err.Error())
		return diags
	}
	plan.fromAPI(t)
	return diags
}

func (r *notificationTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.put(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetNotificationTemplate(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the notification template failed", err.Error())
		return
	}
	state.fromAPI(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *notificationTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if resp.Diagnostics.Append(r.put(ctx, &plan)...); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationTemplateResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, err := r.client.PutNotificationTemplate(ctx, map[string]any{"rules": []client.NotificationStep{}}); err != nil {
		resp.Diagnostics.AddError("Switching the notification template off failed", err.Error())
	}
}

func (r *notificationTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
