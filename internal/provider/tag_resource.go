package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// DefaultTagColor is used when color is not set: POST /api/tags requires one.
const DefaultTagColor = "slate"

var (
	_ resource.ResourceWithConfigure   = &tagResource{}
	_ resource.ResourceWithImportState = &tagResource{}
)

type tagResource struct{ client *client.Client }

// tagModel is shared by the uptimeify_tag resource and data source.
type tagModel struct {
	ID         types.String `tfsdk:"id"`
	PublicID   types.String `tfsdk:"public_id"`
	Name       types.String `tfsdk:"name"`
	Color      types.String `tfsdk:"color"`
	CustomerID types.Int64  `tfsdk:"customer_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
	UpdatedAt  types.String `tfsdk:"updated_at"`
}

func (m *tagModel) fromAPI(t *client.Tag) {
	m.ID = types.StringValue(strconv.FormatInt(t.ID, 10))
	m.PublicID = types.StringValue(t.PublicID)
	m.Name = types.StringValue(t.Name)
	m.Color = types.StringValue(t.Color)
	m.CustomerID = types.Int64PointerValue(t.CustomerID)
	m.CreatedAt = types.StringValue(t.CreatedAt)
	m.UpdatedAt = types.StringValue(t.UpdatedAt)
}

func NewTagResource() resource.Resource { return &tagResource{} }

func (r *tagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *tagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A tag that groups monitors. Import with the numeric id or the public id.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Numeric tag id."},
			"public_id": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Public UUID of the tag."},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Tag name, 1 to 24 characters.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 24)},
			},
			"color": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(DefaultTagColor),
				Description: "Palette key: `" + strings.Join(client.TagColors, "`, `") + "`. Defaults to `" + DefaultTagColor + "`.",
				Validators:  []validator.String{stringvalidator.OneOf(client.TagColors...)},
			},
			"customer_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Scope the tag to this customer. Unset = organization-wide. Customer tokens must set it.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *tagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *tagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tag, err := r.client.CreateTag(ctx, client.TagCreate{
		Name:       plan.Name.ValueString(),
		Color:      plan.Color.ValueString(),
		CustomerID: plan.CustomerID.ValueInt64Pointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating tag failed", err.Error())
		return
	}
	plan.fromAPI(tag)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tag, err := r.client.GetTag(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading tag failed", err.Error())
		return
	}
	state.fromAPI(tag)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state tagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if !plan.Name.Equal(state.Name) {
		patch["name"] = plan.Name.ValueString()
	}
	if !plan.Color.Equal(state.Color) {
		patch["color"] = plan.Color.ValueString()
	}
	if !plan.CustomerID.Equal(state.CustomerID) {
		patch["customerId"] = plan.CustomerID.ValueInt64Pointer() // nil = organization-wide
	}
	tag, err := r.client.UpdateTag(ctx, state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating tag failed", err.Error())
		return
	}
	plan.fromAPI(tag)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTag(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting tag failed", err.Error())
	}
}

func (r *tagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
