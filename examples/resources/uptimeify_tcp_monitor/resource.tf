resource "uptimeify_tcp_monitor" "this" {
  customer_id    = 12
  name           = "Postgres primary"
  hostname       = "db.shop.example"
  port           = 5432
  check_interval = 5
}

# Inverted: alert when the Redis port is reachable from the internet.
resource "uptimeify_tcp_monitor" "redis_closed" {
  customer_id    = 12
  name           = "Redis not exposed"
  hostname       = "cache.shop.example"
  port           = 6379
  expected_state = "down"
}
