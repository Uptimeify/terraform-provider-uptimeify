package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// SMTPTLSModes is the tlsMode enum of smtpPatchSchema.
var SMTPTLSModes = []string{"ssl", "starttls", "none"}

var (
	_ resource.ResourceWithConfigure   = &smtpSettingsResource{}
	_ resource.ResourceWithImportState = &smtpSettingsResource{}
)

type smtpSettingsResource struct{ client *client.Client }

type smtpSettingsModel struct {
	ID                types.String `tfsdk:"id"`
	Host              types.String `tfsdk:"host"`
	Port              types.Int64  `tfsdk:"port"`
	Username          types.String `tfsdk:"username"`
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
	TLSMode           types.String `tfsdk:"tls_mode"`
	FromName          types.String `tfsdk:"from_name"`
	FromEmail         types.String `tfsdk:"from_email"`
	ReplyTo           types.String `tfsdk:"reply_to"`
	HasPassword       types.Bool   `tfsdk:"has_password"`
}

func (m *smtpSettingsModel) fromAPI(s *client.SMTPSettings) {
	m.ID = types.StringValue("smtp")
	m.Host = types.StringPointerValue(s.Host)
	m.Port = types.Int64PointerValue(s.Port)
	m.Username = types.StringPointerValue(s.Username)
	m.TLSMode = types.StringPointerValue(s.TLSMode)
	m.FromName = types.StringPointerValue(s.FromName)
	m.FromEmail = types.StringPointerValue(s.FromEmail)
	m.ReplyTo = types.StringPointerValue(s.ReplyTo)
	m.HasPassword = types.BoolValue(s.HasPassword)
}

func NewSMTPSettingsResource() resource.Resource { return &smtpSettingsResource{} }

func (r *smtpSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smtp_settings"
}

func (r *smtpSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	trimmed := func(min, max int) []validator.String { return []validator.String{stringvalidator.LengthBetween(min, max)} }
	resp.Schema = schema.Schema{
		Description: "The organization's own SMTP server for outgoing mail (one per organization). Destroying it switches " +
			"back to platform sending (`disable`). The host must resolve to a public address. Organization admins only. " +
			"Import with any id, e.g. `smtp`; the password is not imported.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"host":     schema.StringAttribute{Required: true, Validators: trimmed(1, 255)},
			"port":     schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(1, 65535)}},
			"username": schema.StringAttribute{Required: true, Validators: trimmed(1, 200)},
			"password_wo": schema.StringAttribute{Required: true, WriteOnly: true, Sensitive: true, Validators: trimmed(1, 500),
				Description: "Write-only SMTP password, never stored in the state or returned. Sent on create and whenever `password_wo_version` changes."},
			"password_wo_version": schema.Int64Attribute{Optional: true, Description: "Change to resend `password_wo`."},
			"tls_mode": schema.StringAttribute{Optional: true, Validators: []validator.String{stringvalidator.OneOf(SMTPTLSModes...)},
				Description: "`ssl`, `starttls` or `none`."},
			"from_name":    schema.StringAttribute{Optional: true, Validators: trimmed(1, 200)},
			"from_email":   schema.StringAttribute{Required: true, Validators: trimmed(3, 320)},
			"reply_to":     schema.StringAttribute{Optional: true, Validators: trimmed(3, 320)},
			"has_password": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *smtpSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *smtpSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg smtpSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.UpdateSMTP(ctx, map[string]any{
		"host":      plan.Host.ValueString(),
		"port":      plan.Port.ValueInt64(),
		"username":  plan.Username.ValueString(),
		"password":  cfg.PasswordWO.ValueString(),
		"tlsMode":   plan.TLSMode.ValueStringPointer(),
		"fromName":  plan.FromName.ValueStringPointer(),
		"fromEmail": plan.FromEmail.ValueString(),
		"replyTo":   plan.ReplyTo.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Saving SMTP settings failed", err.Error())
		return
	}
	plan.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *smtpSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state smtpSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetSMTP(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading SMTP settings failed", err.Error())
		return
	}
	if s.Host == nil { // disabled outside Terraform: back to platform sending
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *smtpSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg smtpSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Host.Equal(state.Host) {
		patch["host"] = plan.Host.ValueString()
	}
	if !plan.Port.Equal(state.Port) {
		patch["port"] = plan.Port.ValueInt64()
	}
	if !plan.Username.Equal(state.Username) {
		patch["username"] = plan.Username.ValueString()
	}
	if !plan.PasswordWOVersion.Equal(state.PasswordWOVersion) {
		patch["password"] = cfg.PasswordWO.ValueString()
	}
	if !plan.TLSMode.Equal(state.TLSMode) {
		patch["tlsMode"] = plan.TLSMode.ValueStringPointer()
	}
	if !plan.FromName.Equal(state.FromName) {
		patch["fromName"] = plan.FromName.ValueStringPointer()
	}
	if !plan.FromEmail.Equal(state.FromEmail) {
		patch["fromEmail"] = plan.FromEmail.ValueString()
	}
	if !plan.ReplyTo.Equal(state.ReplyTo) {
		patch["replyTo"] = plan.ReplyTo.ValueStringPointer()
	}
	var s *client.SMTPSettings
	var err error
	if len(patch) == 0 { // only the version bookkeeping changed; an empty PATCH is a 400
		s, err = r.client.GetSMTP(ctx)
	} else {
		s, err = r.client.UpdateSMTP(ctx, patch)
	}
	if err != nil {
		resp.Diagnostics.AddError("Saving SMTP settings failed", err.Error())
		return
	}
	plan.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *smtpSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if _, err := r.client.UpdateSMTP(ctx, map[string]any{"disable": true}); err != nil {
		resp.Diagnostics.AddError("Disabling SMTP settings failed", err.Error())
	}
}

func (r *smtpSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
