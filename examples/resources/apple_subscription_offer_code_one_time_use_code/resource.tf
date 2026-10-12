resource "apple_subscription_offer_code" "schools" {
  subscription_id        = "6740021496"
  name                   = "Schools 2026"
  customer_eligibilities = ["NEW"]
  offer_eligibility      = "STACK_WITH_INTRO_OFFERS"
  duration               = "ONE_MONTH"
  offer_mode             = "FREE_TRIAL"
  number_of_periods      = 1
  prices                 = [{ territory_id = "AUS" }]
}

# Apple generates 2,000 codes, each redeemable once. A production batch holds
# 500 to 25,000; for more, add another batch for the same offer.
#
# The codes are not attributes here -- they are secrets. Read them with
# apple_subscription_offer_code_one_time_use_code_values where they are
# distributed, or download them from App Store Connect.
resource "apple_subscription_offer_code_one_time_use_code" "term_1" {
  offer_code_id   = apple_subscription_offer_code.schools.id
  number_of_codes = 2000
  expiration_date = "2027-03-31"
}

# Codes for testing redemption with sandbox accounts: 10 to 10,000.
resource "apple_subscription_offer_code_one_time_use_code" "qa" {
  offer_code_id   = apple_subscription_offer_code.schools.id
  number_of_codes = 10
  environment     = "SANDBOX"
  expiration_date = "2027-01-31"
}
