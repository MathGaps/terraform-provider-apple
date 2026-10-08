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
  # Authentication can be configured via environment variables:
  # APPLE_APP_STORE_CONNECT_ISSUER_ID
  # APPLE_APP_STORE_CONNECT_API_KEY
  # APPLE_APP_STORE_CONNECT_PRIVATE_KEY

  # Or explicitly in the provider block:
  # issuer_id   = "your-issuer-id"
  # api_key     = "your-api-key-id"
  # private_key = file("AuthKey_XXXXXXXXXX.p8")
}