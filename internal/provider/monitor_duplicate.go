package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// allowDuplicateAttr is write-only: it only matters for the create request, so it never shows up
// in state, plans or imports.
var allowDuplicateAttr = schema.BoolAttribute{Optional: true, WriteOnly: true,
	Description: "Create the monitor even if the customer already has one of this type for the same target. " +
		"Without it the create fails with 409 `monitorAlreadyExists` and the error names the import command for the " +
		"existing monitor. Sent with create only."}

// createErrorDetail adds the import command to a 409 monitorAlreadyExists.
func createErrorDetail(err error, typeName string) string {
	id := client.ExistingMonitor(err)
	if id == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s\n\nThe customer already has this monitor (public id %s). To manage it here, import it "+
		"instead of creating a second one (replace NAME with this resource's name):\n\n"+
		"  tofu import %[3]s.NAME %[2]s\n  terraform import %[3]s.NAME %[2]s\n\n"+
		"or add an import block:\n\n  import {\n    to = %[3]s.NAME\n    id = %[2]q\n  }\n\n"+
		"To create a second monitor for the same target anyway, set allow_duplicate = true.", err.Error(), id, typeName)
}
