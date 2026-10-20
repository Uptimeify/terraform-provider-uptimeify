package provider

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// trimmedText refuses surrounding whitespace: the API trims, the state would then differ from the config.
var trimmedText = stringvalidator.RegexMatches(regexp.MustCompile(`^\S(.*\S)?$`), "must not be empty or have surrounding spaces")

// jsonObject checks that a string attribute holds one JSON object.
type jsonObject struct{}

func (jsonObject) Description(context.Context) string         { return "must be a JSON object" }
func (jsonObject) MarkdownDescription(context.Context) string { return "must be a JSON object" }
func (jsonObject) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := parseJSONObject(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", err.Error())
	}
}

func parseJSONObject(s string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// jsonKeep returns prior when it encodes the same JSON as v (key order, spacing), else v encoded.
func jsonKeep(prior types.String, v any) types.String {
	if !prior.IsNull() && !prior.IsUnknown() {
		var p any
		if json.Unmarshal([]byte(prior.ValueString()), &p) == nil && jsonEqual(p, v) {
			return prior
		}
	}
	b, _ := json.Marshal(v)
	return types.StringValue(string(b))
}

// nonEmpty maps nil and "" to null (the API stores blank contact fields as null or "").
func nonEmpty(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// strOrNil is the PATCH value of an optional string: null clears it.
func strOrNil(v types.String) any {
	if v.IsNull() {
		return nil
	}
	return v.ValueString()
}
