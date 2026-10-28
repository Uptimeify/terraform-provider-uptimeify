package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptimeify/terraform-provider-uptimeify/internal/client"
)

// Unit: a 409 monitorAlreadyExists names the import command with the existing public id; any
// other error stays as it is.
func TestCreateErrorDetailNamesImport(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"statusCode":409,"statusMessage":"exists","data":{"code":"monitorAlreadyExists","existingId":7,"existingPublicId":"pub-7"}}`))
	}))
	defer srv.Close()
	_, err := client.New(srv.URL, "wsm_test", "test").CreateMonitor(context.Background(), "tcp-monitors", map[string]any{"allowDuplicate": true})
	got := createErrorDetail(err, "uptimeify_tcp_monitor")
	for _, want := range []string{"tofu import uptimeify_tcp_monitor.NAME pub-7", "terraform import uptimeify_tcp_monitor.NAME pub-7", `id = "pub-7"`, "allow_duplicate = true"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if sent["allowDuplicate"] != true {
		t.Errorf("body %v", sent)
	}
	other := &client.APIError{StatusCode: 409, Code: "ssoDomainTaken"}
	if createErrorDetail(other, "uptimeify_website") != other.Error() {
		t.Error("other 409 got the import hint")
	}
}

// Acceptance: a second monitor on the same target fails with the import hint, and goes through
// with allow_duplicate. Website and TCP cover both create paths.
func TestAccMonitorDuplicate(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	accPreCheck(t)
	n := time.Now().Unix() % 10_000
	name := fmt.Sprintf("tfacc-dup-%d", n)
	url := fmt.Sprintf("https://9.9.9.9/%s", name)
	port := 20_000 + n
	cfg := func(second string) string {
		return accCustomerConfig(t) + fmt.Sprintf(`
resource "uptimeify_website" "a" {
  customer_id = tonumber(data.uptimeify_customer.c.id)
  name        = %[1]q
  url         = %[2]q
  status      = "inactive"
}

resource "uptimeify_tcp_monitor" "a" {
  customer_id = data.uptimeify_customer.c.id
  name        = %[1]q
  hostname    = "9.9.9.9"
  port        = %[3]d
  status      = "disabled"
}
%[4]s`, name, url, port, second)
	}
	dup := func(extra string) string {
		return fmt.Sprintf(`
resource "uptimeify_website" "b" {
  customer_id = tonumber(data.uptimeify_customer.c.id)
  name        = "%[1]s-b"
  url         = %[2]q
  status      = "inactive"
  %[4]s
}

resource "uptimeify_tcp_monitor" "b" {
  customer_id = data.uptimeify_customer.c.id
  name        = "%[1]s-b"
  hostname    = "9.9.9.9"
  port        = %[3]d
  status      = "disabled"
  %[4]s
}
`, name, url, port, extra)
	}
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		Steps: []tfresource.TestStep{
			{Config: cfg("")},
			{
				Config:      cfg(dup("")),
				ExpectError: regexp.MustCompile(`tofu import uptimeify_(website|tcp_monitor)\.NAME`),
			},
			{
				Config: cfg(dup("allow_duplicate = true")),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttrSet("uptimeify_website.b", "public_id"),
					tfresource.TestCheckResourceAttrSet("uptimeify_tcp_monitor.b", "public_id"),
					tfresource.TestCheckNoResourceAttr("uptimeify_website.b", "allow_duplicate"),
				),
			},
		},
	})
}
