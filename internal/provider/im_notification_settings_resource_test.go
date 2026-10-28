package provider

import (
	"context"
	"fmt"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// accIMUserID returns the id of the user accIMUser picked (active admin, editor or responder).
func accIMUserID(t *testing.T) string {
	t.Helper()
	email := accIMUser(t)
	users, err := accClient().ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Email == email {
			return u.ID
		}
	}
	t.Fatalf("user %s not found", email)
	return ""
}

func userNotificationConfig(userID, phone, rules string) string {
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_user_notification_settings" "u" {
  user_id      = %q
  phone_number = %q
  rules        = [%s]
}
`, userID, phone, rules)
}

// The user keeps the settings after destroy; the test restores the chain it found.
func TestAccUserNotificationSettings(t *testing.T) {
	userID := accIMUserID(t)
	ctx := context.Background()
	orig, err := accClient().GetUserNotificationSettings(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	const (
		res    = "uptimeify_user_notification_settings.u"
		phone  = "+491701230000"
		rules1 = `{ urgency = "high", delay_minutes = 0, channel = "email" }, { urgency = "high", delay_minutes = 5, channel = "sms" }`
		rules2 = `{ urgency = "high", delay_minutes = 0, channel = "push" }, { urgency = "low", delay_minutes = 10, channel = "voice" }`
	)
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			if _, err := accClient().GetUserNotificationSettings(ctx, userID); err != nil {
				return fmt.Errorf("settings gone after destroy: %v", err)
			}
			body := map[string]any{"rules": orig.Rules}
			if orig.PhoneNumber != nil {
				body["phoneNumber"] = *orig.PhoneNumber
			} else {
				body["phoneNumber"] = nil
			}
			_, err := accClient().PutUserNotificationSettings(ctx, userID, body)
			return err
		},
		Steps: []tfresource.TestStep{
			{ // create: sms for an unverified number is stored and waits
				Config: userNotificationConfig(userID, phone, rules1),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(res, "id", userID),
					tfresource.TestCheckResourceAttr(res, "phone_number", phone),
					tfresource.TestCheckResourceAttr(res, "phone_verified", "false"),
					tfresource.TestCheckResourceAttr(res, "rules.#", "2"),
					tfresource.TestCheckTypeSetElemNestedAttrs(res, "rules.*", map[string]string{"channel": "sms", "delay_minutes": "5", "urgency": "high"}),
					tfresource.TestCheckResourceAttr(res, "channels.sms", "true"),
					tfresource.TestCheckResourceAttr(res, "channels.push", "false"),
				),
			},
			{ // update: whole chain replaced
				Config: userNotificationConfig(userID, phone, rules2),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(res, "rules.#", "2"),
					tfresource.TestCheckTypeSetElemNestedAttrs(res, "rules.*", map[string]string{"channel": "voice", "delay_minutes": "10", "urgency": "low"}),
					tfresource.TestCheckResourceAttr(res, "channels.email", "false"),
					tfresource.TestCheckResourceAttr(res, "channels.voice", "true"),
				),
			},
			{ResourceName: res, ImportState: true, ImportStateId: userID, ImportStateVerify: true},
			{ // drift: the user (or another admin) changed the chain
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().PutUserNotificationSettings(ctx, userID, map[string]any{
						"rules": []map[string]any{{"urgency": "high", "delayMinutes": 0, "channel": "email"}},
					}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             userNotificationConfig(userID, phone, rules2),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // converge back
				Config: userNotificationConfig(userID, phone, rules2),
				Check:  tfresource.TestCheckTypeSetElemNestedAttrs(res, "rules.*", map[string]string{"channel": "push"}),
			},
		},
	})
}

func notificationTemplateConfig(rules string) string {
	return accProviderBlock + fmt.Sprintf(`
resource "uptimeify_notification_template" "t" {
  rules = [%s]
}
`, rules)
}

// Destroy switches the template off; the test restores the one it found.
func TestAccNotificationTemplate(t *testing.T) {
	_ = accIMUser(t) // activates Incident Management
	ctx := context.Background()
	orig, err := accClient().GetNotificationTemplate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const res = "uptimeify_notification_template.t"
	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: func(*terraform.State) error {
			cur, err := accClient().GetNotificationTemplate(ctx)
			if err != nil {
				return err
			}
			if len(cur.Rules) != 0 {
				return fmt.Errorf("template still has %d rules after destroy", len(cur.Rules))
			}
			if len(orig.Rules) > 0 {
				_, err = accClient().PutNotificationTemplate(ctx, map[string]any{"rules": orig.Rules, "channels": orig.Channels})
			}
			return err
		},
		Steps: []tfresource.TestStep{
			{
				Config: notificationTemplateConfig(`{ urgency = "high", delay_minutes = 0, channel = "push" }, { urgency = "high", delay_minutes = 10, channel = "sms" }`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(res, "id", "org"),
					tfresource.TestCheckResourceAttr(res, "rules.#", "2"),
					tfresource.TestCheckResourceAttr(res, "channels.sms", "true"),
					tfresource.TestCheckResourceAttrSet(res, "updated_at"),
				),
			},
			{
				Config: notificationTemplateConfig(`{ urgency = "low", delay_minutes = 0, channel = "email" }`),
				Check: tfresource.ComposeAggregateTestCheckFunc(
					tfresource.TestCheckResourceAttr(res, "rules.#", "1"),
					tfresource.TestCheckResourceAttr(res, "channels.email", "true"),
					tfresource.TestCheckResourceAttr(res, "channels.push", "false"),
				),
			},
			{ResourceName: res, ImportState: true, ImportStateId: "org", ImportStateVerify: true},
			{ // drift
				PreConfig: func() {
					accPreCheck(t)
					if _, err := accClient().PutNotificationTemplate(ctx, map[string]any{
						"rules": []map[string]any{{"urgency": "high", "delayMinutes": 1, "channel": "push"}},
					}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             notificationTemplateConfig(`{ urgency = "low", delay_minutes = 0, channel = "email" }`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: notificationTemplateConfig(`{ urgency = "low", delay_minutes = 0, channel = "email" }`),
				Check:  tfresource.TestCheckResourceAttr(res, "rules.#", "1"),
			},
		},
	})
}
