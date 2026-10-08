terraform {
  # 1.9 is the floor for referencing another variable from a `validation`
  # block, which is how "every role needs either a CSR or a serial" is
  # enforced before anything reaches Apple.
  required_version = ">= 1.9"

  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}

provider "apple" {
  # Credentials come from the environment:
  #   APPLE_APP_STORE_CONNECT_ISSUER_ID
  #   APPLE_APP_STORE_CONNECT_API_KEY
  #   APPLE_APP_STORE_CONNECT_PRIVATE_KEY
}

locals {
  # The two identities a normal iOS project needs. Development signs debug
  # builds onto registered devices; distribution signs everything that leaves
  # the machine, both Ad Hoc and App Store.
  certificate_types = {
    development  = "IOS_DEVELOPMENT"
    distribution = "IOS_DISTRIBUTION"
  }

  # Development and Ad Hoc profiles are only meaningful once at least one
  # device is registered, so they are skipped on a devices-free configuration.
  has_devices = length(var.devices) > 0

  # Every role this configuration knows about, however it was obtained. Both
  # maps are keyed by role, and a role may legitimately appear in neither: a
  # CI-only release pipeline usually declares distribution and nothing else.
  certificate_roles = distinct(concat(
    keys(var.csr_contents),
    keys(var.adopt_certificate_serials),
  ))

  has_development = contains(local.certificate_roles, "development")

  # A role is adopted when an existing serial number is given for it, and issued
  # from the supplied CSR otherwise. Adopted certificates are read, never
  # created, so an apply cannot revoke the certificate a team is still shipping
  # with.
  issued_certificate_types = {
    for role, type in local.certificate_types :
    role => type
    if contains(keys(var.csr_contents), role) && !contains(keys(var.adopt_certificate_serials), role)
  }

  # Issued and adopted certificates in one shape, so that profiles and outputs
  # do not have to care which is which.
  #
  # nonsensitive() unwraps certificate_content deliberately. The provider marks
  # that attribute sensitive, but an X.509 certificate is a public document:
  # Apple hands it to every member of the team, and it is inert without the
  # private key, which this module never holds. Leaving it wrapped would force
  # the outputs to be sensitive and redact values that are safe to print.
  certificates = merge(
    {
      for role, certificate in apple_certificate.signing : role => {
        id                  = certificate.id
        certificate_type    = certificate.certificate_type
        certificate_content = nonsensitive(certificate.certificate_content)
        expiration_date     = certificate.expiration_date
      }
    },
    {
      for role, adopted in data.apple_certificates.adopted : role => {
        id                  = one(adopted.certificates).id
        certificate_type    = one(adopted.certificates).certificate_type
        certificate_content = nonsensitive(one(adopted.certificates).certificate_content)
        expiration_date     = one(adopted.certificates).expiration_date
      }
    },
  )

  # count-based profiles come back as lists; the App Store profile is always
  # present. Flattening them here keeps the profiles output the same shape
  # whether or not devices are configured.
  all_profiles = concat(
    apple_profile.development,
    apple_profile.ad_hoc,
    [apple_profile.app_store],
  )
}

# --- App ID and entitlements ------------------------------------------------

resource "apple_bundle_id" "app" {
  identifier = var.bundle_identifier
  name       = var.app_name
  platform   = "IOS"
}

# Capabilities that need no further configuration. Anything with settings
# (ICLOUD, APP_GROUPS, APPLE_PAY, ASSOCIATED_DOMAINS) takes nested `settings`
# blocks and belongs in your own configuration -- see
# examples/resources/apple_bundle_id_capability.
resource "apple_bundle_id_capability" "app" {
  for_each = var.capabilities

  bundle_id       = apple_bundle_id.app.id
  capability_type = each.value
}

# --- Devices ----------------------------------------------------------------

# Adding a device here regenerates every profile that includes devices, because
# `devices` forces replacement on apple_profile. That is the behaviour fastlane
# match needs `--force_for_new_devices` for; here it is just what the next plan
# does.
resource "apple_device" "team" {
  for_each = var.devices

  name     = each.key
  udid     = each.value
  platform = "IOS"
}

# --- Signing identities -----------------------------------------------------

# The private key is generated on your machine and never reaches Terraform.
# Only the CSR is configured here, and a CSR carries a public key and a
# signature over it -- nothing secret. That is what keeps this state, and
# everything it outputs, free of key material:
#
#   openssl genrsa -out distribution.key 2048
#   openssl req -new -key distribution.key -out distribution.csr \
#       -subj "/CN=My App distribution"
#
# See README.md for the Keychain Access equivalent, which produces a CSR
# without the key ever becoming a file.
resource "apple_certificate" "signing" {
  for_each = local.issued_certificate_types

  certificate_type    = each.value
  csr_content         = var.csr_contents[each.key]
  early_renewal_hours = var.early_renewal_hours
}

# Certificates the team already has. Reading one rather than importing it is
# deliberate: `csr_content` forces replacement on apple_certificate, and Apple
# does not reliably return the CSR it was issued from, so an imported
# certificate is reissued on the next apply -- and issuing a replacement revokes
# the original, breaking every build already signed with it.
data "apple_certificates" "adopted" {
  for_each = var.adopt_certificate_serials

  serial_number = each.value

  lifecycle {
    postcondition {
      condition     = length(self.certificates) == 1
      error_message = "No certificate in this Apple team has the serial number given in var.adopt_certificate_serials."
    }
  }
}

# --- Provisioning profiles --------------------------------------------------

# Profiles snapshot the App ID's entitlements at the moment Apple generates
# them, so they must be created after the capabilities they are meant to carry.
resource "apple_profile" "development" {
  count = local.has_devices && local.has_development ? 1 : 0

  name         = "${var.profile_name_prefix} Development"
  profile_type = "IOS_APP_DEVELOPMENT"
  bundle_id    = apple_bundle_id.app.id
  certificates = [local.certificates["development"].id]
  devices      = [for device in apple_device.team : device.id]

  depends_on = [apple_bundle_id_capability.app]
}

resource "apple_profile" "ad_hoc" {
  count = local.has_devices ? 1 : 0

  name         = "${var.profile_name_prefix} Ad Hoc"
  profile_type = "IOS_APP_ADHOC"
  bundle_id    = apple_bundle_id.app.id
  certificates = [local.certificates["distribution"].id]
  devices      = [for device in apple_device.team : device.id]

  depends_on = [apple_bundle_id_capability.app]
}

# App Store profiles carry no devices at all.
resource "apple_profile" "app_store" {
  name         = "${var.profile_name_prefix} App Store"
  profile_type = "IOS_APP_STORE"
  bundle_id    = apple_bundle_id.app.id
  certificates = [local.certificates["distribution"].id]

  depends_on = [apple_bundle_id_capability.app]
}
