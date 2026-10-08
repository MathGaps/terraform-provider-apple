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

# Example CSR content (you would generate this using openssl or similar tools)
# This is a placeholder - in practice, you'd generate a real CSR
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

# Create an iOS Development certificate
resource "apple_certificate" "ios_development" {
  certificate_type = "IOS_DEVELOPMENT"
  csr_content      = local.example_csr
}

# Create an iOS Distribution certificate that replaces itself 30 days before it
# expires. The window is evaluated during `terraform plan`, so this only takes
# effect when Terraform runs -- schedule a periodic plan/apply if you rely on it.
resource "apple_certificate" "ios_distribution" {
  certificate_type    = "IOS_DISTRIBUTION"
  csr_content         = local.example_csr
  early_renewal_hours = 720 # 30 days

  # Certificates are immutable, so renewal is a replacement. Create the new
  # certificate before revoking the old one to avoid a signing gap.
  lifecycle {
    create_before_destroy = true
  }
}

# Create a Mac App Development certificate
resource "apple_certificate" "mac_development" {
  certificate_type = "MAC_APP_DEVELOPMENT"
  csr_content      = local.example_csr
}

# Create a Developer ID Application certificate
resource "apple_certificate" "developer_id_application" {
  certificate_type = "DEVELOPER_ID_APPLICATION"
  csr_content      = local.example_csr
}

# Output certificate information (excluding sensitive certificate content)
output "ios_development_certificate" {
  value = {
    id               = apple_certificate.ios_development.id
    serial_number    = apple_certificate.ios_development.serial_number
    display_name     = apple_certificate.ios_development.display_name
    name             = apple_certificate.ios_development.name
    certificate_type = apple_certificate.ios_development.certificate_type
    platform         = apple_certificate.ios_development.platform
    expiration_date  = apple_certificate.ios_development.expiration_date
    requester_email  = apple_certificate.ios_development.requester_email
  }
}

output "ios_distribution_certificate" {
  value = {
    id                = apple_certificate.ios_distribution.id
    serial_number     = apple_certificate.ios_distribution.serial_number
    display_name      = apple_certificate.ios_distribution.display_name
    certificate_type  = apple_certificate.ios_distribution.certificate_type
    platform          = apple_certificate.ios_distribution.platform
    expiration_date   = apple_certificate.ios_distribution.expiration_date
    ready_for_renewal = apple_certificate.ios_distribution.ready_for_renewal
  }
}

output "mac_development_certificate" {
  value = {
    id               = apple_certificate.mac_development.id
    serial_number    = apple_certificate.mac_development.serial_number
    display_name     = apple_certificate.mac_development.display_name
    certificate_type = apple_certificate.mac_development.certificate_type
    expiration_date  = apple_certificate.mac_development.expiration_date
  }
}

output "developer_id_certificate" {
  value = {
    id               = apple_certificate.developer_id_application.id
    serial_number    = apple_certificate.developer_id_application.serial_number
    display_name     = apple_certificate.developer_id_application.display_name
    certificate_type = apple_certificate.developer_id_application.certificate_type
    expiration_date  = apple_certificate.developer_id_application.expiration_date
  }
}

# Output all certificate serial numbers for reference
output "all_certificate_serials" {
  value = {
    ios_development  = apple_certificate.ios_development.serial_number
    ios_distribution = apple_certificate.ios_distribution.serial_number
    mac_development  = apple_certificate.mac_development.serial_number
    developer_id_app = apple_certificate.developer_id_application.serial_number
  }
}

# Note: The certificate_content attribute contains the actual certificate data
# but is marked as sensitive and won't appear in outputs. You can access it
# in other resources or data sources that need the certificate content.