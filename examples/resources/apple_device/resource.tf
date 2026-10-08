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

# Register an iOS device
resource "apple_device" "my_iphone" {
  name     = "My iPhone 15 Pro"
  udid     = "00008130-000A1B2C3D4E5F60" # 8-16 hex, reported by iPhone XS and later
  platform = "IOS"
}

# Register an iPad
resource "apple_device" "my_ipad" {
  name     = "Development iPad"
  udid     = "00008027-001122334455667E" # 8-16 hex, reported by A12 and later iPads
  platform = "IOS"
}

# Register a Mac device
resource "apple_device" "my_mac" {
  name     = "MacBook Pro Development"
  udid     = "550e8400-e29b-41d4-a716-446655440000" # UUID format for Mac devices
  platform = "MAC_OS"
}

# Register an Apple TV
resource "apple_device" "my_apple_tv" {
  name     = "Living Room Apple TV"
  udid     = "12345678-1234-1234-1234-123456789012" # UUID format for Apple TV
  platform = "TV_OS"
}

# Register an Apple Watch (via paired iPhone)
resource "apple_device" "my_watch" {
  name     = "Apple Watch Series 9"
  udid     = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678" # 40-character hex, reported by iPhone X and earlier
  platform = "IOS"                                      # Apple Watch devices are registered under iOS platform
}

# Register an Apple Vision Pro
resource "apple_device" "my_vision_pro" {
  name     = "Vision Pro Dev Unit"
  udid     = "7c9e6679-7425-40de-944b-e07fc1f90ae7" # UUID format for Vision devices
  platform = "VISION_OS"
}

# Output device information
output "device_ids" {
  description = "Apple-assigned IDs for registered devices"
  value = {
    iphone     = apple_device.my_iphone.id
    ipad       = apple_device.my_ipad.id
    mac        = apple_device.my_mac.id
    apple_tv   = apple_device.my_apple_tv.id
    watch      = apple_device.my_watch.id
    vision_pro = apple_device.my_vision_pro.id
  }
}

output "device_details" {
  description = "Detailed information about registered devices"
  value = {
    iphone = {
      name         = apple_device.my_iphone.name
      udid         = apple_device.my_iphone.udid
      platform     = apple_device.my_iphone.platform
      device_class = apple_device.my_iphone.device_class
      model        = apple_device.my_iphone.model
      status       = apple_device.my_iphone.status
      added_date   = apple_device.my_iphone.added_date
    }
    mac = {
      name         = apple_device.my_mac.name
      udid         = apple_device.my_mac.udid
      platform     = apple_device.my_mac.platform
      device_class = apple_device.my_mac.device_class
      model        = apple_device.my_mac.model
      status       = apple_device.my_mac.status
      added_date   = apple_device.my_mac.added_date
    }
  }
}

# Example of importing an existing device
# To import: terraform import apple_device.existing_device <device_id_or_udid>
resource "apple_device" "existing_device" {
  name     = "Previously Registered Device"
  udid     = "00008110-000E4C8A0C50401E" # The UDID the device already registered under
  platform = "IOS"

  # This resource can be imported using either the Apple device ID or UDID
  # terraform import apple_device.existing_device 00008110-000E4C8A0C50401E
  # or
  # terraform import apple_device.existing_device ABC123DEF456
}

# Example using data source to find and reference existing devices
data "apple_devices" "existing_ios_devices" {
  platform = "IOS"
  status   = "ENABLED"
}

# Local value showing how to work with existing devices
locals {
  # Get UDIDs of all enabled iOS devices
  enabled_ios_udids = [
    for device in data.apple_devices.existing_ios_devices.devices : device.udid
  ]

  # Create a map of device names to UDIDs
  device_name_to_udid = {
    for device in data.apple_devices.existing_ios_devices.devices :
    device.name => device.udid
  }
}