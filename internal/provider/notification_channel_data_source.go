package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &notificationChannelDataSource{}
	_ datasource.DataSourceWithConfigValidators = &notificationChannelDataSource{}
)

type notificationChannelDataSource struct{ client *client.Client }

func NewNotificationChannelDataSource() datasource.DataSource {
	return &notificationChannelDataSource{}
}

func (d *notificationChannelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_channel"
}

func (d *notificationChannelDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	(&notificationChannelResource{}).Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := dataSourceAttributes(rs.Schema, "secret_config_wo_version", "allowed_package_types")
	attrs["allowed_package_types"] = dschema.SetAttribute{Computed: true, ElementType: types.StringType}
	attrs["id"] = dschema.StringAttribute{Optional: true, Computed: true, Description: "Numeric channel id."}
	attrs["name"] = dschema.StringAttribute{Optional: true, Computed: true}
	attrs["customer_id"] = dschema.Int64Attribute{Optional: true, Computed: true,
		Description: "With `name`: search this customer's channels too (default: organization channels only)."}
	resp.Schema = dschema.Schema{
		Description: "Look up one notification channel by `id` or by `name` (exactly one). Fails when no or several " +
			"channels carry the name. Secrets are never returned (see `secrets_set`). Website overrides are not found by name.",
		Attributes: attrs,
	}
}

func (d *notificationChannelDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *notificationChannelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *notificationChannelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg notificationChannelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.NotificationChannel
	if !cfg.ID.IsNull() {
		ch, err := d.client.GetNotificationChannel(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading notification channel failed", err.Error())
			return
		}
		found = ch
	} else {
		customer := ""
		if !cfg.CustomerID.IsNull() {
			customer = strconv.FormatInt(cfg.CustomerID.ValueInt64(), 10)
		}
		all, err := d.client.ListNotificationChannels(ctx, customer)
		if err != nil {
			resp.Diagnostics.AddError("Listing notification channels failed", err.Error())
			return
		}
		var matches []client.NotificationChannel
		for _, ch := range all {
			if ch.Name == cfg.Name.ValueString() && ch.WebsiteID == nil {
				matches = append(matches, ch)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddError("Notification channel lookup by name failed",
				fmt.Sprintf("%d channels are named %q, expected exactly 1. Use id instead.", len(matches), cfg.Name.ValueString()))
			return
		}
		found = &matches[0]
	}
	resp.Diagnostics.Append(cfg.fromAPI(ctx, found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
