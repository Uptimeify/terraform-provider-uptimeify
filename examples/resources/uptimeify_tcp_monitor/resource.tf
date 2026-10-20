resource "uptimeify_tcp_monitor" "this" {
  customer_id    = 12
  name           = "Postgres primary"
  hostname       = "db.shop.example"
  port           = 5432
  check_interval = 5
}
