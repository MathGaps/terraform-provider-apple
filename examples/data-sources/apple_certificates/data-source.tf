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

# Query all certificates
data "apple_certificates" "all" {
  # No filters - returns all certificates
}

# Query only iOS Development certificates
data "apple_certificates" "ios_development" {
  certificate_type = "IOS_DEVELOPMENT"
  platform         = "IOS"
  sort_by          = "display_name"
  sort_order       = "asc"
}

# Query certificates by multiple types
data "apple_certificates" "distribution_certificates" {
  certificate_types = [
    "IOS_DISTRIBUTION",
    "MAC_APP_DISTRIBUTION"
  ]
  sort_by    = "expiration_date"
  sort_order = "desc"
  limit      = 10
}

# Query certificates with name pattern
data "apple_certificates" "production_certificates" {
  name_pattern = "*Production*"
  sort_by      = "certificate_type"
}

# Query certificates by specific serial number
data "apple_certificates" "specific_certificate" {
  serial_number = "A1B2C3D4E5F6"
  limit         = 1
}

# Output all certificates
output "all_certificates" {
  value = [for cert in data.apple_certificates.all.certificates : {
    id               = cert.id
    display_name     = cert.display_name
    certificate_type = cert.certificate_type
    serial_number    = cert.serial_number
    expiration_date  = cert.expiration_date
    platform         = cert.platform
  }]
}

# Output iOS Development certificates
output "ios_dev_certificates" {
  value = {
    count       = length(data.apple_certificates.ios_development.certificates)
    total_count = data.apple_certificates.ios_development.total_count
    certificates = [for cert in data.apple_certificates.ios_development.certificates : {
      display_name    = cert.display_name
      serial_number   = cert.serial_number
      expiration_date = cert.expiration_date
    }]
  }
}

# Output distribution certificates
output "distribution_certificates" {
  value = {
    filtered_count = data.apple_certificates.distribution_certificates.filtered_count
    certificates = [for cert in data.apple_certificates.distribution_certificates.certificates : {
      display_name     = cert.display_name
      certificate_type = cert.certificate_type
      expiration_date  = cert.expiration_date
    }]
  }
}

# Output production certificates
output "production_certificates" {
  value = [for cert in data.apple_certificates.production_certificates.certificates : {
    display_name     = cert.display_name
    certificate_type = cert.certificate_type
    requester_email  = cert.requester_email
  }]
}

# Output specific certificate details (sensitive data excluded)
output "specific_certificate" {
  value = length(data.apple_certificates.specific_certificate.certificates) > 0 ? {
    found            = true
    display_name     = data.apple_certificates.specific_certificate.certificates[0].display_name
    certificate_type = data.apple_certificates.specific_certificate.certificates[0].certificate_type
    expiration_date  = data.apple_certificates.specific_certificate.certificates[0].expiration_date
    } : {
    found = false
  }
}