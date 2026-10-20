package provider

import (
	"context"
	"strconv"

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
	_ resource.ResourceWithConfigure   = &imOrgSettingsResource{}
	_ resource.ResourceWithImportState = &imOrgSettingsResource{}
)

// MonitoringBridgeModes is the monitoringBridge enum of im-channels.ts.
var MonitoringBridgeModes = []string{"im_only", "classic_only", "both"}

type imOrgSettingsResource struct{ client *client.Client }

type imOrgSettingsModel struct {
	ID                types.String `tfsdk:"id"`
	OrganizationID    types.Int64  `tfsdk:"organization_id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	OverageEnabled    types.Bool   `tfsdk:"overage_enabled"`
	MonitoringBridge  types.String `tfsdk:"monitoring_bridge"`
	FallbackChannelID types.Int64  `tfsdk:"fallback_channel_id"`
}

func (m *imOrgSettingsModel) fromAPI(s *client.ImOrgSettings) {
	m.ID = types.StringValue(strconv.FormatInt(s.OrganizationID, 10))
	m.OrganizationID = types.Int64Value(s.OrganizationID)
	m.Enabled = types.BoolValue(s.Enabled)
	m.OverageEnabled = types.BoolValue(s.OverageEnabled)
	m.MonitoringBridge = types.StringValue(s.MonitoringBridge)
	m.FallbackChannelID = types.Int64PointerValue(s.FallbackChannelID)
}

func NewImOrgSettingsResource() resource.Resource { return &imOrgSettingsResource{} }

func (r *imOrgSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_im_org_settings"
}

func (r *imOrgSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Incident Management settings of the token's organization (one per organization). Creating it " +
			"adopts the existing settings and applies the configured values; destroying it only removes it from the " +
			"state, the settings stay. Import with any id, e.g. `org`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The organization id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"organization_id": schema.Int64Attribute{Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"enabled": schema.BoolAttribute{Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Description: "Whether Incident Management is active (`POST /api/im/activate`; read only here)."},
			"overage_enabled": schema.BoolAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Description:   "Allow Incident Management usage beyond the plan (billed as overage). Unset = keep the current value."},
			"monitoring_bridge": schema.StringAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Where classic monitoring alerts go: `im_only`, `classic_only` or `both`. Unset = keep the current value.",
				Validators:    []validator.String{stringvalidator.OneOf(MonitoringBridgeModes...)}},
			"fallback_channel_id": schema.Int64Attribute{Optional: true,
				Description: "Active Incident Management channel paged when an escalation runs out. Unset clears it.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)}},
		},
	}
}

func (r *imOrgSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// apply PATCHes the planned values that differ from the current settings (an empty PATCH is a 400).
func (r *imOrgSettingsResource) apply(ctx context.Context, plan *imOrgSettingsModel) error {
	cur, err := r.client.GetImOrgSettings(ctx)
	if err != nil {
		return err
	}
	patch := map[string]any{}
	if v := plan.OverageEnabled; !v.IsUnknown() && !v.IsNull() && v.ValueBool() != cur.OverageEnabled {
		patch["overageEnabled"] = v.ValueBool()
	}
	if v := plan.MonitoringBridge; !v.IsUnknown() && !v.IsNull() && v.ValueString() != cur.MonitoringBridge {
		patch["monitoringBridge"] = v.ValueString()
	}
	if !plan.FallbackChannelID.Equal(types.Int64PointerValue(cur.FallbackChannelID)) {
		patch["fallbackChannelId"] = plan.FallbackChannelID.ValueInt64Pointer()
	}
	if len(patch) > 0 {
		if cur, err = r.client.UpdateImOrgSettings(ctx, patch); err != nil {
			return err
		}
	}
	plan.fromAPI(cur)
	return nil
}

func (r *imOrgSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan imOrgSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Updating Incident Management settings failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *imOrgSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state imOrgSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetImOrgSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading Incident Management settings failed", err.Error())
		return
	}
	state.fromAPI(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *imOrgSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan imOrgSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Updating Incident Management settings failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the settings: they exist as long as the organization does.
func (r *imOrgSettingsResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *imOrgSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
