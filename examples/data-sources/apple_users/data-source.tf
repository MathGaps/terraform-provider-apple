terraform {
  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}

provider "apple" {
  # Configuration will be supplied via environment variables:
  # APPLE_APP_STORE_CONNECT_ISSUER_ID
  # APPLE_APP_STORE_CONNECT_API_KEY
  # APPLE_APP_STORE_CONNECT_PRIVATE_KEY
}

# Everybody under Users and Access. Pending invitations are not here: Apple keeps
# them in a separate collection until they are accepted.
data "apple_users" "all" {}

# Who can approve a release.
data "apple_users" "admins" {
  role       = "ADMIN"
  sort_by    = "username"
  sort_order = "asc"
}

# Roles is an any-of rather than an exact match, because most people hold
# several.
data "apple_users" "builders" {
  roles = ["DEVELOPER", "APP_MANAGER"]
}

# Everybody on one domain who can issue signing certificates — the audit the
# examples/signing module's certificates are worth running beside.
data "apple_users" "signers" {
  username_pattern     = "@example\\.com$"
  provisioning_allowed = true
}

# Members restricted to a named set of apps, with the apps they can see.
data "apple_users" "restricted" {
  all_apps_visible = false
}

output "team_size" {
  value = data.apple_users.all.total_count
}

output "signers" {
  value = [for user in data.apple_users.signers.users : user.username]
}

output "restricted_visibility" {
  value = {
    for user in data.apple_users.restricted.users :
    user.username => user.visible_apps
  }
}
