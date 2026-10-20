package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

var (
	imStopModes = []string{"acknowledged", "resolved"}
	hhmmRe      = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	hhmm        = stringvalidator.RegexMatches(hhmmRe, `must be "HH:mm", 00:00 to 23:59`)
	isoWeekday  = int64validator.Between(1, 7)
)

// parseTwoIDs reads an import id "<a>/<b>" of two positive integers.
func parseTwoIDs(id, want string) (int64, int64, error) {
	parts := strings.Split(id, "/")
	if len(parts) == 2 {
		a, errA := strconv.ParseInt(parts[0], 10, 64)
		b, errB := strconv.ParseInt(parts[1], 10, 64)
		if errA == nil && errB == nil && a > 0 && b > 0 {
			return a, b, nil
		}
	}
	return 0, 0, fmt.Errorf("expected %s, e.g. 12/34; got %q", want, id)
}

func idPair(a, b int64) string { return fmt.Sprintf("%d/%d", a, b) }

// weeklyWindowModel is one { from, until, days } window.
type weeklyWindowModel struct {
	From  types.String `tfsdk:"from"`
	Until types.String `tfsdk:"until"`
	Days  []int64      `tfsdk:"days"`
}

var weeklyWindowAttrTypes = map[string]attr.Type{
	"from": types.StringType, "until": types.StringType, "days": types.ListType{ElemType: types.Int64Type},
}

// weeklyWindowsAttribute is Optional+Computed with default [], the API stores an absent list as [].
func weeklyWindowsAttribute(desc string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:    true,
		Computed:    true,
		Default:     listdefault.StaticValue(types.ListValueMust(types.ObjectType{AttrTypes: weeklyWindowAttrTypes}, nil)),
		Description: desc,
		Validators:  []validator.List{listvalidator.SizeAtMost(50)},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"from":  schema.StringAttribute{Required: true, Description: "`HH:mm`.", Validators: []validator.String{hhmm}},
			"until": schema.StringAttribute{Required: true, Description: "`HH:mm` or `24:00`.", Validators: []validator.String{stringvalidator.Any(hhmm, stringvalidator.OneOf("24:00"))}},
			"days": schema.ListAttribute{ElementType: types.Int64Type, Required: true, Description: "ISO weekdays, 1 = Monday to 7 = Sunday.",
				Validators: []validator.List{listvalidator.ValueInt64sAre(isoWeekday)}},
		}},
	}
}

func weeklyFromAPI(in []client.WeeklyWindow) []weeklyWindowModel {
	out := []weeklyWindowModel{}
	for _, w := range in {
		days := w.Days
		if days == nil {
			days = []int64{}
		}
		out = append(out, weeklyWindowModel{From: types.StringValue(w.From), Until: types.StringValue(w.Until), Days: days})
	}
	return out
}

func weeklyToAPI(in []weeklyWindowModel) []client.WeeklyWindow {
	out := []client.WeeklyWindow{}
	for _, w := range in {
		days := w.Days
		if days == nil {
			days = []int64{}
		}
		out = append(out, client.WeeklyWindow{From: w.From.ValueString(), Until: w.Until.ValueString(), Days: days})
	}
	return out
}

func sameWeekly(a, b []weeklyWindowModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].From.Equal(b[i].From) || !a[i].Until.Equal(b[i].Until) || !sameInt64s(a[i].Days, b[i].Days) {
			return false
		}
	}
	return true
}

// setStrings turns a set attribute into []string (nil for null/unknown).
func setStrings(ctx context.Context, s types.Set) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	s.ElementsAs(ctx, &out, false)
	return out
}

func stringSetValue(in []string) types.Set {
	elems := make([]attr.Value, 0, len(in))
	for _, v := range in {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}
