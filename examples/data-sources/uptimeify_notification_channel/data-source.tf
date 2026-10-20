data "uptimeify_notification_channel" "ops" {
  name = "Ops mail"
}

data "uptimeify_notification_channel" "by_id" {
  id = "42"
}
