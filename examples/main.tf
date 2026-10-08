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

# Create a new Bundle ID
resource "apple_bundle_id" "example_app" {
  identifier = "com.example.myapp"
  name       = "My Example App"
  platform   = "IOS"
}

# Create Merchant IDs for Apple Pay functionality
resource "apple_merchant_id" "example_store" {
  identifier   = "merchant.com.example.myapp"
  display_name = "My Example App Store"
}

resource "apple_merchant_id" "premium_store" {
  identifier   = "merchant.com.example.myapp.premium"
  display_name = "My Example App Premium Store"
}

resource "apple_merchant_id" "subscriptions" {
  identifier   = "merchant.com.example.myapp.subscriptions"
  display_name = "My Example App Subscriptions"
}

# Add capabilities to the Bundle ID
resource "apple_bundle_id_capability" "push_notifications" {
  bundle_id       = apple_bundle_id.example_app.id
  capability_type = "PUSH_NOTIFICATIONS"
}

resource "apple_bundle_id_capability" "in_app_purchase" {
  bundle_id       = apple_bundle_id.example_app.id
  capability_type = "IN_APP_PURCHASE"
}

resource "apple_bundle_id_capability" "apple_pay" {
  bundle_id       = apple_bundle_id.example_app.id
  capability_type = "APPLE_PAY"

  # Reference the created Merchant ID
  settings = [
    {
      key   = "APPLE_PAY_IDENTIFIERS"
      value = apple_merchant_id.example_store.identifier
    },
  ]
}

resource "apple_bundle_id_capability" "icloud" {
  bundle_id       = apple_bundle_id.example_app.id
  capability_type = "ICLOUD"

  settings = [
    {
      key   = "ICLOUD_VERSION"
      value = "XCODE_5"
    },
    {
      key   = "ICLOUD_SERVICES"
      value = "CloudKit"
    },
  ]
}

# Query existing Bundle IDs
data "apple_bundle_ids" "ios_apps" {
  platform = "IOS"
}

# Query Bundle ID capabilities
data "apple_bundle_id_capabilities" "example_app_capabilities" {
  bundle_id = apple_bundle_id.example_app.id
}

# Query all Merchant IDs
data "apple_merchant_ids" "all_merchants" {
  depends_on = [
    apple_merchant_id.example_store,
    apple_merchant_id.premium_store,
    apple_merchant_id.subscriptions,
  ]
}

# Query Merchant IDs with identifier prefix
data "apple_merchant_ids" "example_merchants" {
  identifier_prefix = "merchant.com.example"
  sort_by           = "display_name"
  sort_order        = "asc"
}

# Query Merchant IDs with display name pattern
data "apple_merchant_ids" "store_merchants" {
  display_name_pattern = ".*Store.*"
}

# ============================================================================
# PASS TYPE IDs (Apple Wallet Pass Management)
# ============================================================================

# Create Pass Type IDs for different types of passes
resource "apple_pass_type_id" "loyalty_card" {
  identifier = "pass.com.example.loyalty"
  name       = "Example Store Loyalty Card"
}

resource "apple_pass_type_id" "event_tickets" {
  identifier = "pass.com.example.events"
  name       = "Event Ticket Passes"
}

resource "apple_pass_type_id" "store_card" {
  identifier = "pass.com.example.storecard"
  name       = "Store Card Passes"
}

resource "apple_pass_type_id" "coupon_pass" {
  identifier = "pass.com.example.coupons"
  name       = "Promotional Coupons"
}

# Query all Pass Type IDs
data "apple_pass_type_ids" "all_pass_types" {
  depends_on = [
    apple_pass_type_id.loyalty_card,
    apple_pass_type_id.event_tickets,
    apple_pass_type_id.store_card,
    apple_pass_type_id.coupon_pass,
  ]
}

# Query Pass Type IDs with identifier prefix
data "apple_pass_type_ids" "example_passes" {
  identifier_prefix = "pass.com.example"
  sort_by           = "name"
  sort_order        = "asc"
}

# Query Pass Type IDs with name pattern
data "apple_pass_type_ids" "card_passes" {
  name_pattern = ".*Card.*"
  sort_by      = "identifier"
}

# Example CSR for certificate creation
locals {
  example_csr = <<-EOT
    -----BEGIN CERTIFICATE REQUEST-----
    MIICljCCAX4CAQAwUTELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNBMRYwFAYDVQQH
    DA1TYW4gRnJhbmNpc2NvMR0wGwYDVQQKDBRFeGFtcGxlIENvbXBhbnksIEluYzCC
    ASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAM7K2+Xk3nWoQ5j+9fGOZjXf
    ... (truncated for example - use a real CSR)
    -----END CERTIFICATE REQUEST-----
  EOT
}

# Create certificates for development and distribution
resource "apple_certificate" "ios_development" {
  certificate_type = "IOS_DEVELOPMENT"
  csr_content      = local.example_csr
}

resource "apple_certificate" "ios_distribution" {
  certificate_type = "IOS_DISTRIBUTION"
  csr_content      = local.example_csr
}

# Create Pass Type ID certificates for Wallet passes
resource "apple_certificate" "loyalty_pass_cert" {
  certificate_type = "PASS_TYPE_ID"
  csr_content      = local.example_csr
}

resource "apple_certificate" "nfc_pass_cert" {
  certificate_type = "PASS_TYPE_ID_WITH_NFC"
  csr_content      = local.example_csr
}

# Query existing certificates
data "apple_certificates" "development_certificates" {
  certificate_type = "IOS_DEVELOPMENT"
  platform         = "IOS"
  sort_by          = "expiration_date"
  sort_order       = "desc"
  limit            = 5
}

# Query all certificates with name pattern
data "apple_certificates" "production_certificates" {
  name_pattern = "*Production*"
  sort_by      = "display_name"
}

# Query Pass Type ID certificates
data "apple_certificates" "pass_certificates" {
  certificate_type = "PASS_TYPE_ID"
  sort_by          = "expiration_date"
  sort_order       = "desc"
}

# Query NFC-enabled Pass Type ID certificates
data "apple_certificates" "nfc_pass_certificates" {
  certificate_type = "PASS_TYPE_ID_WITH_NFC"
  sort_by          = "display_name"
}

# Register development devices
resource "apple_device" "dev_iphone" {
  name     = "Development iPhone"
  udid     = "00008030000a4d8e0ab8802e1234567890abcdef" # Replace with actual UDID
  platform = "IOS"
}

resource "apple_device" "dev_ipad" {
  name     = "Development iPad"
  udid     = "00008027000c1d2e3f4a5b6c7d8e9f0a1b2c3d4e" # Replace with actual UDID
  platform = "IOS"
}

resource "apple_device" "test_mac" {
  name     = "Test MacBook"
  udid     = "550e8400-e29b-41d4-a716-446655440000" # Replace with actual UDID
  platform = "MAC_OS"
}

# Query existing devices
data "apple_devices" "all_devices" {}

# Query iOS devices only
data "apple_devices" "ios_devices" {
  platform   = "IOS"
  sort_by    = "name"
  sort_order = "asc"
}

# Query enabled devices by device class
data "apple_devices" "iphones" {
  device_class = "IPHONE"
  status       = "ENABLED"
}

# ============================================================================
# PROVISIONING PROFILES
# ============================================================================

# Create a development profile for the Bundle ID
resource "apple_profile" "example_development" {
  name         = "Development Profile for ${apple_bundle_id.example_app.name}"
  profile_type = "IOS_APP_DEVELOPMENT"
  bundle_id    = apple_bundle_id.example_app.id
  certificates = [apple_certificate.ios_development.id]
  devices      = [apple_device.dev_iphone.id, apple_device.dev_ipad.id]
}

# Create an App Store distribution profile
resource "apple_profile" "example_app_store" {
  name         = "App Store Profile for ${apple_bundle_id.example_app.name}"
  profile_type = "IOS_APP_STORE"
  bundle_id    = apple_bundle_id.example_app.id
  certificates = [apple_certificate.ios_distribution.id]
  # Note: No devices for App Store distribution profiles
}

# Create an Ad Hoc distribution profile
resource "apple_profile" "example_adhoc" {
  name         = "Ad Hoc Profile for ${apple_bundle_id.example_app.name}"
  profile_type = "IOS_APP_ADHOC"
  bundle_id    = apple_bundle_id.example_app.id
  certificates = [apple_certificate.ios_distribution.id]
  devices      = [apple_device.dev_iphone.id, apple_device.dev_ipad.id]
}

# Query all profiles
data "apple_profiles" "all_profiles" {
  depends_on = [
    apple_profile.example_development,
    apple_profile.example_app_store,
    apple_profile.example_adhoc,
  ]
}

# Query active iOS profiles
data "apple_profiles" "active_ios_profiles" {
  platform      = "IOS"
  profile_state = "ACTIVE"
  sort_by       = "created_date"
  sort_order    = "desc"
}

# Query development profiles by name pattern
data "apple_profiles" "development_profiles" {
  name_pattern = ".*Development.*"
  platform     = "IOS"
}

# Output the new Bundle ID details
output "new_bundle_id" {
  value = {
    id         = apple_bundle_id.example_app.id
    identifier = apple_bundle_id.example_app.identifier
    name       = apple_bundle_id.example_app.name
  }
}

# Output created Merchant IDs
output "created_merchant_ids" {
  value = {
    example_store = {
      id           = apple_merchant_id.example_store.id
      identifier   = apple_merchant_id.example_store.identifier
      display_name = apple_merchant_id.example_store.display_name
    }
    premium_store = {
      id           = apple_merchant_id.premium_store.id
      identifier   = apple_merchant_id.premium_store.identifier
      display_name = apple_merchant_id.premium_store.display_name
    }
    subscriptions = {
      id           = apple_merchant_id.subscriptions.id
      identifier   = apple_merchant_id.subscriptions.identifier
      display_name = apple_merchant_id.subscriptions.display_name
    }
  }
}

# Output Merchant ID statistics
output "merchant_id_statistics" {
  value = {
    total_merchants   = data.apple_merchant_ids.all_merchants.total_count
    example_merchants = data.apple_merchant_ids.example_merchants.filtered_count
    store_merchants   = data.apple_merchant_ids.store_merchants.filtered_count
  }
}

# Output example Merchant IDs details
output "example_merchant_ids" {
  value = [for mid in data.apple_merchant_ids.example_merchants.merchant_ids : {
    identifier   = mid.identifier
    display_name = mid.display_name
  }]
}

# Output store-related Merchant IDs
output "store_merchant_ids" {
  value = [for mid in data.apple_merchant_ids.store_merchants.merchant_ids : {
    identifier   = mid.identifier
    display_name = mid.display_name
  }]
}

# Output Bundle ID capabilities
output "bundle_id_capabilities" {
  value = [for cap in data.apple_bundle_id_capabilities.example_app_capabilities.capabilities : {
    capability_type = cap.capability_type
    settings_count  = length(cap.settings)
  }]
}

# Output existing Bundle IDs
output "existing_ios_bundle_ids" {
  value = [for bid in data.apple_bundle_ids.ios_apps.bundle_ids : {
    identifier = bid.identifier
    name       = bid.name
    platform   = bid.platform
  }]
}

# Output certificate details (excluding sensitive content)
output "created_certificates" {
  value = {
    ios_development = {
      id               = apple_certificate.ios_development.id
      serial_number    = apple_certificate.ios_development.serial_number
      display_name     = apple_certificate.ios_development.display_name
      certificate_type = apple_certificate.ios_development.certificate_type
      expiration_date  = apple_certificate.ios_development.expiration_date
      platform         = apple_certificate.ios_development.platform
    }
    ios_distribution = {
      id               = apple_certificate.ios_distribution.id
      serial_number    = apple_certificate.ios_distribution.serial_number
      display_name     = apple_certificate.ios_distribution.display_name
      certificate_type = apple_certificate.ios_distribution.certificate_type
      expiration_date  = apple_certificate.ios_distribution.expiration_date
      platform         = apple_certificate.ios_distribution.platform
    }
  }
}

# Output development certificates from data source
output "development_certificates" {
  value = {
    total_count    = data.apple_certificates.development_certificates.total_count
    filtered_count = data.apple_certificates.development_certificates.filtered_count
    certificates = [for cert in data.apple_certificates.development_certificates.certificates : {
      display_name    = cert.display_name
      serial_number   = cert.serial_number
      expiration_date = cert.expiration_date
      requester_email = cert.requester_email
    }]
  }
}

# Output production certificates
output "production_certificates" {
  value = [for cert in data.apple_certificates.production_certificates.certificates : {
    display_name     = cert.display_name
    certificate_type = cert.certificate_type
    serial_number    = cert.serial_number
    expiration_date  = cert.expiration_date
  }]
}

# Output registered devices
output "registered_devices" {
  value = {
    dev_iphone = {
      id           = apple_device.dev_iphone.id
      name         = apple_device.dev_iphone.name
      udid         = apple_device.dev_iphone.udid
      platform     = apple_device.dev_iphone.platform
      device_class = apple_device.dev_iphone.device_class
      status       = apple_device.dev_iphone.status
    }
    test_mac = {
      id           = apple_device.test_mac.id
      name         = apple_device.test_mac.name
      udid         = apple_device.test_mac.udid
      platform     = apple_device.test_mac.platform
      device_class = apple_device.test_mac.device_class
      status       = apple_device.test_mac.status
    }
  }
}

# Output device statistics
output "device_statistics" {
  value = {
    total_devices    = data.apple_devices.all_devices.total_count
    ios_device_count = data.apple_devices.ios_devices.filtered_count
    iphone_count     = data.apple_devices.iphones.filtered_count

    devices_by_platform = {
      for platform in ["IOS", "MAC_OS", "TV_OS", "WATCH_OS", "VISION_OS"] :
      platform => length([
        for device in data.apple_devices.all_devices.devices :
        device if device.platform == platform
      ])
    }

    devices_by_status = {
      for status in ["ENABLED", "PROCESSING", "INELIGIBLE"] :
      status => length([
        for device in data.apple_devices.all_devices.devices :
        device if device.status == status
      ])
    }
  }
}

# Output iOS device details
output "ios_device_details" {
  value = [
    for device in data.apple_devices.ios_devices.devices : {
      name         = device.name
      device_class = device.device_class
      model        = device.model
      status       = device.status
      added_date   = device.added_date
    }
  ]
}

# Output created profiles
output "created_profiles" {
  value = {
    development_profile = {
      id                = apple_profile.example_development.id
      name              = apple_profile.example_development.name
      uuid              = apple_profile.example_development.uuid
      profile_state     = apple_profile.example_development.profile_state
      profile_type      = apple_profile.example_development.profile_type
      created_date      = apple_profile.example_development.created_date
      expiration_date   = apple_profile.example_development.expiration_date
      device_count      = length(apple_profile.example_development.devices)
      certificate_count = length(apple_profile.example_development.certificates)
    }

    app_store_profile = {
      id              = apple_profile.example_app_store.id
      name            = apple_profile.example_app_store.name
      uuid            = apple_profile.example_app_store.uuid
      profile_state   = apple_profile.example_app_store.profile_state
      profile_type    = apple_profile.example_app_store.profile_type
      created_date    = apple_profile.example_app_store.created_date
      expiration_date = apple_profile.example_app_store.expiration_date
    }

    adhoc_profile = {
      id                = apple_profile.example_adhoc.id
      name              = apple_profile.example_adhoc.name
      uuid              = apple_profile.example_adhoc.uuid
      profile_state     = apple_profile.example_adhoc.profile_state
      profile_type      = apple_profile.example_adhoc.profile_type
      created_date      = apple_profile.example_adhoc.created_date
      expiration_date   = apple_profile.example_adhoc.expiration_date
      device_count      = length(apple_profile.example_adhoc.devices)
      certificate_count = length(apple_profile.example_adhoc.certificates)
    }
  }
}

# Output profile statistics
output "profile_statistics" {
  value = {
    total_profiles       = data.apple_profiles.all_profiles.total_count
    active_ios_profiles  = data.apple_profiles.active_ios_profiles.filtered_count
    development_profiles = data.apple_profiles.development_profiles.filtered_count
  }
}

# Output active iOS profiles
output "active_ios_profiles" {
  value = [for profile in data.apple_profiles.active_ios_profiles.profiles : {
    name            = profile.name
    uuid            = profile.uuid
    profile_type    = profile.profile_type
    profile_state   = profile.profile_state
    expiration_date = profile.expiration_date
  }]
}

# Output development profiles
output "development_profiles" {
  value = [for profile in data.apple_profiles.development_profiles.profiles : {
    name         = profile.name
    uuid         = profile.uuid
    profile_type = profile.profile_type
    created_date = profile.created_date
  }]
}

# ============================================================================
# PASS TYPE ID OUTPUTS
# ============================================================================

# Output created Pass Type IDs
output "created_pass_type_ids" {
  value = {
    loyalty_card = {
      id         = apple_pass_type_id.loyalty_card.id
      identifier = apple_pass_type_id.loyalty_card.identifier
      name       = apple_pass_type_id.loyalty_card.name
    }
    event_tickets = {
      id         = apple_pass_type_id.event_tickets.id
      identifier = apple_pass_type_id.event_tickets.identifier
      name       = apple_pass_type_id.event_tickets.name
    }
    store_card = {
      id         = apple_pass_type_id.store_card.id
      identifier = apple_pass_type_id.store_card.identifier
      name       = apple_pass_type_id.store_card.name
    }
    coupon_pass = {
      id         = apple_pass_type_id.coupon_pass.id
      identifier = apple_pass_type_id.coupon_pass.identifier
      name       = apple_pass_type_id.coupon_pass.name
    }
  }
}

# Output Pass Type ID statistics
output "pass_type_id_statistics" {
  value = {
    total_pass_types   = data.apple_pass_type_ids.all_pass_types.total_count
    example_pass_types = data.apple_pass_type_ids.example_passes.filtered_count
    card_pass_types    = data.apple_pass_type_ids.card_passes.filtered_count
  }
}

# Output example Pass Type IDs details
output "example_pass_type_ids" {
  value = [for pass in data.apple_pass_type_ids.example_passes.pass_type_ids : {
    identifier = pass.identifier
    name       = pass.name
  }]
}

# Output card-related Pass Type IDs
output "card_pass_type_ids" {
  value = [for pass in data.apple_pass_type_ids.card_passes.pass_type_ids : {
    identifier = pass.identifier
    name       = pass.name
  }]
}

# Output Pass Type ID certificates (excluding sensitive content)
output "pass_certificates" {
  value = {
    loyalty_pass_cert = {
      id               = apple_certificate.loyalty_pass_cert.id
      serial_number    = apple_certificate.loyalty_pass_cert.serial_number
      display_name     = apple_certificate.loyalty_pass_cert.display_name
      certificate_type = apple_certificate.loyalty_pass_cert.certificate_type
      expiration_date  = apple_certificate.loyalty_pass_cert.expiration_date
    }
    nfc_pass_cert = {
      id               = apple_certificate.nfc_pass_cert.id
      serial_number    = apple_certificate.nfc_pass_cert.serial_number
      display_name     = apple_certificate.nfc_pass_cert.display_name
      certificate_type = apple_certificate.nfc_pass_cert.certificate_type
      expiration_date  = apple_certificate.nfc_pass_cert.expiration_date
    }
  }
}

# Output Pass Type ID certificate statistics
output "pass_certificate_statistics" {
  value = {
    pass_certificates     = data.apple_certificates.pass_certificates.filtered_count
    nfc_pass_certificates = data.apple_certificates.nfc_pass_certificates.filtered_count
  }
}

# Example configuration for pass generation (JSON output)
output "pass_generation_config" {
  value = {
    for key, pass_type_id in {
      loyalty_card  = apple_pass_type_id.loyalty_card
      event_tickets = apple_pass_type_id.event_tickets
      store_card    = apple_pass_type_id.store_card
      coupon_pass   = apple_pass_type_id.coupon_pass
      } : key => {
      passTypeIdentifier = pass_type_id.identifier
      description        = pass_type_id.name
      formatVersion      = 1
      organizationName   = "Example Company"
      teamIdentifier     = "TEAM123456" # Replace with your team identifier
    }
  }
}