events {
  workers = 4;
}

options {
  pidfile = "/var/run/rspamd/rspamd.pid";
  cache_file = "/var/lib/rspamd/rspamd_dynamic";
}

# Listen for Postfix milter connections
worker "rspamd_proxy" {
  bind_socket = "0.0.0.0:11332";
  count = 4;
  upstream "local" {
    default = yes;
    self_scan = yes;
  }
}

# DKIM signing — configure after you have DNS keys
# dkim_signing {
#   path = "/var/lib/rspamd/dkim/$domain.$selector.key";
#   selector = "mail";
# }
