package provider

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The last failed TXT verification of a custom domain (status page and white label alike), kept by
// the API until a verification succeeds. Read-only; changes outside Terraform, hence no plan modifier.

func lastVerificationErrorAttr() schema.StringAttribute {
	return schema.StringAttribute{Computed: true, Description: "JSON diagnosis of the last failed TXT verification " +
		"(`recordName`, `expected`, `lookup`, `observed`, `nameservers`, `authoritative`, `propagation`), " +
		"decode with `jsondecode()`; null when the last verification succeeded or none failed yet."}
}

func lastVerificationErrorAtAttr() schema.StringAttribute {
	return schema.StringAttribute{Computed: true, Description: "When `last_verification_error` happened (RFC 3339)."}
}

// rawJSONOrNull keeps the API's JSON object as a string, null for an absent or JSON null value.
func rawJSONOrNull(raw json.RawMessage) types.String {
	if len(raw) == 0 || string(raw) == "null" {
		return types.StringNull()
	}
	return types.StringValue(string(raw))
}
