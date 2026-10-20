package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	_ resource.ResourceWithConfigure   = &monitorTagResource{}
	_ resource.ResourceWithImportState = &monitorTagResource{}
)

type monitorTagResource struct{ client *client.Client }

type monitorTagModel struct {
	ID          types.String `tfsdk:"id"`
	MonitorType types.String `tfsdk:"monitor_type"`
	MonitorID   types.Int64  `tfsdk:"monitor_id"`
	TagID       types.Int64  `tfsdk:"tag_id"`
}

func (m *monitorTagModel) ref() client.MonitorTagRef {
	return client.MonitorTagRef{MonitorType: m.MonitorType.ValueString(), MonitorID: m.MonitorID.ValueInt64(), TagID: m.TagID.ValueInt64()}
}

func monitorTagID(r client.MonitorTagRef) string {
	return fmt.Sprintf("%s/%d/%d", r.MonitorType, r.MonitorID, r.TagID)
}

func NewMonitorTagResource() resource.Resource { return &monitorTagResource{} }

func (r *monitorTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_tag"
}

func (r *monitorTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceInt := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("Puts one tag on one monitor. A monitor carries at most %d tags. "+
			"Import with `<monitor_type>/<monitor_id>/<tag_id>`.", client.MaxTagsPerMonitor),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "`<monitor_type>/<monitor_id>/<tag_id>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"monitor_type": schema.StringAttribute{
				Required:      true,
				Description:   "One of `" + strings.Join(client.TagMonitorTypes, "`, `") + "`.",
				Validators:    []validator.String{stringvalidator.OneOf(client.TagMonitorTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monitor_id": schema.Int64Attribute{Required: true, Description: "Numeric id of the monitor.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: replaceInt},
			"tag_id": schema.Int64Attribute{Required: true, Description: "Numeric tag id.",
				Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: replaceInt},
		},
	}
}

func (r *monitorTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *monitorTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ref := plan.ref()
	if _, err := r.client.AddMonitorTag(ctx, ref); err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "tagLimitReached" {
			resp.Diagnostics.AddError("Tag limit reached",
				fmt.Sprintf("%s monitor %d already carries %d tags, the maximum. Remove a tag from it first. (%s)",
					ref.MonitorType, ref.MonitorID, client.MaxTagsPerMonitor, err))
			return
		}
		resp.Diagnostics.AddError("Adding tag to monitor failed", err.Error())
		return
	}
	plan.ID = types.StringValue(monitorTagID(ref))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tags, err := r.client.ListMonitorTags(ctx, state.MonitorType.ValueString(), state.MonitorID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Reading monitor tags failed", err.Error())
		return
	}
	// The list is empty for a missing monitor, so a gone monitor or tag both read as "not assigned".
	if !slices.ContainsFunc(tags, func(t client.Tag) bool { return t.ID == state.TagID.ValueInt64() }) {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(monitorTagID(state.ref()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: every attribute requires replacement.
func (r *monitorTagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, req.Plan.Raw)...)
}

func (r *monitorTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// 404 = tag or monitor already gone, so is the assignment.
	if err := r.client.RemoveMonitorTag(ctx, state.ref()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Removing tag from monitor failed", err.Error())
	}
}

func (r *monitorTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	var monitorID, tagID int64
	var err1, err2 error
	if len(parts) == 3 {
		monitorID, err1 = strconv.ParseInt(parts[1], 10, 64)
		tagID, err2 = strconv.ParseInt(parts[2], 10, 64)
	}
	if len(parts) != 3 || !slices.Contains(client.TagMonitorTypes, parts[0]) || err1 != nil || err2 != nil || monitorID < 1 || tagID < 1 {
		resp.Diagnostics.AddError("Invalid import id",
			fmt.Sprintf("Expected <monitor_type>/<monitor_id>/<tag_id>, e.g. website/12/34; got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monitor_type"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monitor_id"), monitorID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tag_id"), tagID)...)
}
