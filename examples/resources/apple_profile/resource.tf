terraform {
  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}

# Configure the Apple Provider
provider "apple" {
  # Authentication via environment variables:
  # APPLE_APP_STORE_CONNECT_ISSUER_ID
  # APPLE_APP_STORE_CONNECT_API_KEY
  # APPLE_APP_STORE_CONNECT_PRIVATE_KEY
}

# Get existing Bundle IDs for iOS
data "apple_bundle_ids" "ios" {
  platform = "IOS"
  limit    = 1
}

# Get existing development certificates
data "apple_certificates" "development" {
  certificate_type = "IOS_DEVELOPMENT"
  platform         = "IOS"
  limit            = 1
}

# Get existing distribution certificates
data "apple_certificates" "distribution" {
  certificate_type = "IOS_DISTRIBUTION"
  platform         = "IOS"
  limit            = 1
}

# Get existing iOS devices
data "apple_devices" "ios" {
  platform = "IOS"
  status   = "ENABLED"
  limit    = 10
}

# Create a development profile.
# profile_type selects both the platform and the distribution method; the
# computed `platform` attribute is derived from it by Apple.
resource "apple_profile" "development" {
  name         = "iOS Development Profile Example"
  profile_type = "IOS_APP_DEVELOPMENT"
  bundle_id    = data.apple_bundle_ids.ios.bundle_ids[0].id
  certificates = [data.apple_certificates.development.certificates[0].id]
  devices      = data.apple_devices.ios.devices[*].id
}

# Create an App Store distribution profile (no devices needed)
resource "apple_profile" "app_store" {
  name         = "iOS App Store Distribution Profile"
  profile_type = "IOS_APP_STORE"
  bundle_id    = data.apple_bundle_ids.ios.bundle_ids[0].id
  certificates = [data.apple_certificates.distribution.certificates[0].id]
  # Note: devices are not specified for App Store distribution
}

# Get macOS Bundle IDs and certificates for macOS profile example
data "apple_bundle_ids" "macos" {
  platform = "MAC_OS"
  limit    = 1
}

data "apple_certificates" "macos_development" {
  certificate_type = "MAC_APP_DEVELOPMENT"
  platform         = "MAC_OS"
  limit            = 1
}

data "apple_devices" "macos" {
  platform = "MAC_OS"
  status   = "ENABLED"
  limit    = 5
}

# Create a macOS development profile (conditional on data availability)
resource "apple_profile" "macos_development" {
  count = length(data.apple_bundle_ids.macos.bundle_ids) > 0 && length(data.apple_certificates.macos_development.certificates) > 0 ? 1 : 0

  name         = "macOS Development Profile Example"
  profile_type = "MAC_APP_DEVELOPMENT"
  bundle_id    = data.apple_bundle_ids.macos.bundle_ids[0].id
  certificates = [data.apple_certificates.macos_development.certificates[0].id]
  devices      = data.apple_devices.macos.devices[*].id
}

# Example with multiple certificates (Ad Hoc distribution)
data "apple_certificates" "adhoc" {
  certificate_type = "IOS_DISTRIBUTION"
  platform         = "IOS"
  limit            = 2
}

resource "apple_profile" "adhoc_distribution" {
  count = length(data.apple_certificates.adhoc.certificates) >= 1 ? 1 : 0

  name         = "iOS Ad Hoc Distribution Profile"
  profile_type = "IOS_APP_ADHOC"
  bundle_id    = data.apple_bundle_ids.ios.bundle_ids[0].id
  certificates = data.apple_certificates.adhoc.certificates[*].id
  devices      = data.apple_devices.ios.devices[*].id
}

# Output profile information
output "development_profile" {
  value = {
    id              = apple_profile.development.id
    name            = apple_profile.development.name
    uuid            = apple_profile.development.uuid
    profile_state   = apple_profile.development.profile_state
    profile_type    = apple_profile.development.profile_type
    created_date    = apple_profile.development.created_date
    expiration_date = apple_profile.development.expiration_date
    platform        = apple_profile.development.platform
  }
}

output "app_store_profile" {
  value = {
    id              = apple_profile.app_store.id
    name            = apple_profile.app_store.name
    uuid            = apple_profile.app_store.uuid
    profile_state   = apple_profile.app_store.profile_state
    profile_type    = apple_profile.app_store.profile_type
    created_date    = apple_profile.app_store.created_date
    expiration_date = apple_profile.app_store.expiration_date
    platform        = apple_profile.app_store.platform
  }
}

output "macos_profile" {
  value = length(apple_profile.macos_development) > 0 ? {
    id              = apple_profile.macos_development[0].id
    name            = apple_profile.macos_development[0].name
    uuid            = apple_profile.macos_development[0].uuid
    profile_state   = apple_profile.macos_development[0].profile_state
    profile_type    = apple_profile.macos_development[0].profile_type
    created_date    = apple_profile.macos_development[0].created_date
    expiration_date = apple_profile.macos_development[0].expiration_date
    platform        = apple_profile.macos_development[0].platform
  } : null
}

output "adhoc_profile" {
  value = length(apple_profile.adhoc_distribution) > 0 ? {
    id              = apple_profile.adhoc_distribution[0].id
    name            = apple_profile.adhoc_distribution[0].name
    uuid            = apple_profile.adhoc_distribution[0].uuid
    profile_state   = apple_profile.adhoc_distribution[0].profile_state
    profile_type    = apple_profile.adhoc_distribution[0].profile_type
    created_date    = apple_profile.adhoc_distribution[0].created_date
    expiration_date = apple_profile.adhoc_distribution[0].expiration_date
    platform        = apple_profile.adhoc_distribution[0].platform
  } : null
}

# Summary output
output "profiles_summary" {
  value = {
    development_profile_created = true
    app_store_profile_created   = true
    macos_profile_created       = length(apple_profile.macos_development) > 0
    adhoc_profile_created       = length(apple_profile.adhoc_distribution) > 0

    total_profiles_created = 2 + length(apple_profile.macos_development) + length(apple_profile.adhoc_distribution)

    bundle_id_used = data.apple_bundle_ids.ios.bundle_ids[0].identifier
    devices_count  = length(data.apple_devices.ios.devices)
  }
}

# Profile content information (Base64 data excluded for security).
# profile_content is a sensitive attribute, so values derived from it must be
# declared sensitive too.
output "profile_metadata" {
  sensitive = true
  value = {
    development = {
      has_profile_content = apple_profile.development.profile_content != ""
      certificate_count   = length(apple_profile.development.certificates)
      device_count        = length(apple_profile.development.devices)
    }
    app_store = {
      has_profile_content = apple_profile.app_store.profile_content != ""
      certificate_count   = length(apple_profile.app_store.certificates)
      device_count        = length(apple_profile.app_store.devices)
    }
  }
}