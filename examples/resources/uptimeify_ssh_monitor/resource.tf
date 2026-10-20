resource "uptimeify_ssh_monitor" "this" {
  customer_id = 12
  name        = "Bastion"
  hostname    = "bastion.shop.example"
  username    = "monitor"

  private_key_wo     = file("~/.ssh/uptimeify_monitor")
  secrets_wo_version = 1
}

# Port reachability only, no credentials.
resource "uptimeify_ssh_monitor" "port_only" {
  customer_id = 12
  name        = "Bastion port"
  hostname    = "bastion.shop.example"
  check_mode  = "tcp"
}
