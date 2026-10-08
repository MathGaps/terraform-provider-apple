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

# Get all devices in your Apple Developer account
data "apple_devices" "all" {}

# Filter devices by platform
data "apple_devices" "ios_devices" {
  platform   = "IOS"
  sort_by    = "name"
  sort_order = "asc"
}

# Filter devices by multiple platforms. Apple Watches are registered under the
# IOS platform, distinguished by their APPLE_WATCH device class.
data "apple_devices" "mobile_devices" {
  platforms = ["IOS", "VISION_OS"]
  sort_by   = "added_date"
}

# Filter devices by device class
data "apple_devices" "iphones" {
  device_class = "IPHONE"
  limit        = 10
}

# Filter devices using patterns
data "apple_devices" "test_devices" {
  name_pattern = ".*Test.*"
  status       = "ENABLED"
  sort_by      = "name"
}

# Get recent devices (limited)
data "apple_devices" "recent" {
  limit      = 5
  sort_by    = "added_date"
  sort_order = "desc"
}

# Output examples
output "total_devices" {
  description = "Total number of devices in the account"
  value       = data.apple_devices.all.total_count
}

output "ios_device_count" {
  description = "Number of iOS devices"
  value       = data.apple_devices.ios_devices.filtered_count
}

output "device_names" {
  description = "Names of all devices"
  value       = [for device in data.apple_devices.all.devices : device.name]
}

output "iphone_details" {
  description = "Details of iPhone devices"
  value = [
    for device in data.apple_devices.iphones.devices : {
      name         = device.name
      udid         = device.udid
      model        = device.model
      status       = device.status
      device_class = device.device_class
    }
  ]
}

output "enabled_devices_by_platform" {
  description = "Count of enabled devices by platform"
  value = {
    for platform in ["IOS", "MAC_OS", "TV_OS", "VISION_OS"] :
    platform => length([
      for device in data.apple_devices.all.devices :
      device if device.platform == platform && device.status == "ENABLED"
    ])
  }
}