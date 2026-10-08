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

# Query all profiles
data "apple_profiles" "all" {}

# Query iOS profiles only
data "apple_profiles" "ios_profiles" {
  platform = "IOS"
  sort_by  = "name"
}

# Query profiles by multiple platforms
data "apple_profiles" "mobile_profiles" {
  platforms  = ["IOS", "TV_OS", "WATCH_OS"]
  sort_by    = "platform"
  sort_order = "asc"
}

# Query active profiles only
data "apple_profiles" "active_profiles" {
  profile_state = "ACTIVE"
  sort_by       = "expiration_date"
  sort_order    = "asc"
  limit         = 20
}

# Query development profiles
data "apple_profiles" "development_profiles" {
  profile_type = "IOS_APP_DEVELOPMENT"
  sort_by      = "created_date"
  sort_order   = "desc"
}

# Query profiles by name pattern
data "apple_profiles" "production_profiles" {
  name_pattern = ".*[Pp]roduction.*"
  sort_by      = "name"
}

# Query App Store distribution profiles
data "apple_profiles" "app_store_profiles" {
  profile_type = "IOS_APP_STORE"
  platform     = "IOS"
}

# Query macOS profiles
data "apple_profiles" "macos_profiles" {
  platform = "MAC_OS"
  limit    = 10
}

# Query profiles with sorting by expiration date (most recent first)
data "apple_profiles" "recent_profiles" {
  sort_by    = "expiration_date"
  sort_order = "desc"
  limit      = 5
}

# Output all profiles summary
output "all_profiles_summary" {
  value = {
    total_count    = data.apple_profiles.all.total_count
    filtered_count = data.apple_profiles.all.filtered_count
  }
}

# Output iOS profiles
output "ios_profiles" {
  value = [for profile in data.apple_profiles.ios_profiles.profiles : {
    id              = profile.id
    name            = profile.name
    platform        = profile.platform
    profile_state   = profile.profile_state
    profile_type    = profile.profile_type
    uuid            = profile.uuid
    created_date    = profile.created_date
    expiration_date = profile.expiration_date
  }]
}

# Output mobile platforms profile count
output "mobile_profile_stats" {
  value = {
    total_count     = data.apple_profiles.mobile_profiles.filtered_count
    platforms_found = distinct([for profile in data.apple_profiles.mobile_profiles.profiles : profile.platform])
    profile_types   = distinct([for profile in data.apple_profiles.mobile_profiles.profiles : profile.profile_type])
  }
}

# Output active profiles summary
output "active_profiles_summary" {
  value = {
    count = data.apple_profiles.active_profiles.filtered_count
    profiles = [for profile in data.apple_profiles.active_profiles.profiles : {
      name            = profile.name
      platform        = profile.platform
      profile_type    = profile.profile_type
      expiration_date = profile.expiration_date
    }]
  }
}

# Output development profiles details
output "development_profiles_details" {
  value = [for profile in data.apple_profiles.development_profiles.profiles : {
    name          = profile.name
    uuid          = profile.uuid
    created_date  = profile.created_date
    profile_state = profile.profile_state
  }]
}

# Output production profiles
output "production_profiles" {
  value = {
    count = data.apple_profiles.production_profiles.filtered_count
    profiles = [for profile in data.apple_profiles.production_profiles.profiles : {
      name         = profile.name
      profile_type = profile.profile_type
      uuid         = profile.uuid
    }]
  }
}

# Output App Store profiles
output "app_store_profiles" {
  value = {
    count = data.apple_profiles.app_store_profiles.filtered_count
    names = [for profile in data.apple_profiles.app_store_profiles.profiles : profile.name]
  }
}

# Output macOS profiles
output "macos_profiles" {
  value = [for profile in data.apple_profiles.macos_profiles.profiles : {
    name         = profile.name
    profile_type = profile.profile_type
    uuid         = profile.uuid
  }]
}

# Output most recently created profiles
output "recent_profiles" {
  value = [for profile in data.apple_profiles.recent_profiles.profiles : {
    name            = profile.name
    platform        = profile.platform
    created_date    = profile.created_date
    expiration_date = profile.expiration_date
    profile_state   = profile.profile_state
  }]
}

# Profile statistics by platform
output "profile_statistics" {
  value = {
    by_platform = {
      for platform in ["IOS", "MAC_OS", "TV_OS", "WATCH_OS"] :
      platform => length([
        for profile in data.apple_profiles.all.profiles :
        profile if profile.platform == platform
      ])
    }

    by_state = {
      for state in ["ACTIVE", "INVALID", "EXPIRED"] :
      state => length([
        for profile in data.apple_profiles.all.profiles :
        profile if profile.profile_state == state
      ])
    }

    by_type = {
      for type in distinct([for profile in data.apple_profiles.all.profiles : profile.profile_type]) :
      type => length([
        for profile in data.apple_profiles.all.profiles :
        profile if profile.profile_type == type
      ]) if type != null && type != ""
    }
  }
}