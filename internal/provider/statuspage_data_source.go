package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &statusPageDataSource{}
	_ datasource.DataSourceWithConfigValidators = &statusPageDataSource{}
)

type statusPageDataSource struct{ client *client.Client }

func NewStatusPageDataSource() datasource.DataSource { return &statusPageDataSource{} }

func (d *statusPageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (d *statusPageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "Look up one status page by `id`, `slug` or `name` (exactly one). Fails when no or several visible pages carry the name.",
		Attributes: map[string]schema.Attribute{
			"id":                         schema.StringAttribute{Optional: true, Computed: true, Description: "Numeric id or public id."},
			"slug":                       schema.StringAttribute{Optional: true, Computed: true},
			"name":                       schema.StringAttribute{Optional: true, Computed: true},
			"public_id":                  c(""),
			"organization_id":            schema.Int64Attribute{Computed: true},
			"customer_id":                schema.Int64Attribute{Computed: true},
			"customer_name":              c(""),
			"description":                c(""),
			"visibility":                 c("`public`, `customer_members_only`, `password_protected` or `sso_members_only`."),
			"management_type":            c("`managed` or `self_service`."),
			"is_published":               schema.BoolAttribute{Computed: true},
			"subscriptions_enabled":      schema.BoolAttribute{Computed: true},
			"subscription_delay_minutes": schema.Int64Attribute{Computed: true},
			"include_customer_monitors":  schema.BoolAttribute{Computed: true, Description: "Every monitor of the customer appears after the components."},
			"monitor_updates_enabled":    schema.BoolAttribute{Computed: true, Description: "Monitors write to the page (state, incidents, subscriber notifications); `false` = only manual reports and explicitly linked incidents."},
			"hidden_monitors": schema.SetNestedAttribute{Computed: true, Description: "Monitors of the customer the page does not show.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{Computed: true},
					"id":   schema.Int64Attribute{Computed: true},
				}}},
			"sms_subscriptions_enabled":  schema.BoolAttribute{Computed: true},
			"push_subscriptions_enabled": schema.BoolAttribute{Computed: true},
			"ip_allowlist":               schema.SetAttribute{ElementType: types.StringType, Computed: true, Description: "CIDR ranges allowed to see the page; empty = no IP restriction."},
			"has_password":               schema.BoolAttribute{Computed: true},
			"access_password_set_at":     c(""),
			"created_at":                 c(""),
			"updated_at":                 c(""),
		},
	}
}

func (d *statusPageDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("slug"), path.MatchRoot("name"))}
}

func (d *statusPageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *statusPageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg statusPageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.StatusPage
	if !cfg.ID.IsNull() {
		p, err := d.client.GetStatusPage(ctx, cfg.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading status page failed", err.Error())
			return
		}
		found = p
	} else {
		pages, err := d.client.ListStatusPages(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing status pages failed", err.Error())
			return
		}
		key, want := "name", cfg.Name.ValueString()
		if !cfg.Slug.IsNull() {
			key, want = "slug", cfg.Slug.ValueString()
		}
		var matches []client.StatusPage
		for _, p := range pages {
			if (key == "slug" && p.Slug == want) || (key == "name" && p.Name == want) {
				matches = append(matches, p)
			}
		}
		if len(matches) != 1 {
			resp.Diagnostics.AddError("Status page lookup failed",
				fmt.Sprintf("%d visible status pages have %s %q, expected exactly 1. Use id instead.", len(matches), key, want))
			return
		}
		found = &matches[0]
	}
	configuredID := cfg.ID
	cfg.fromAPI(found)
	if !configuredID.IsNull() {
		cfg.ID = configuredID // keep a configured public id as written
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
