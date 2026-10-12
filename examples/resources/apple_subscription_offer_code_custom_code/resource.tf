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

# One string many teachers type: letters and digits only, up to 64. Up to
# 25,000 redemptions. An expiration date is optional; when set it is at most six
# months away, so it is left out here rather than written as a date that passes.
resource "apple_subscription_offer_code_custom_code" "school2026" {
  offer_code_id   = apple_subscription_offer_code.schools.id
  custom_code     = "SCHOOL2026"
  number_of_codes = 5000
}

# Ending it early: active = false stops redemptions in place, for good, and
# frees the string for another offer of the app.
resource "apple_subscription_offer_code_custom_code" "pilot" {
  offer_code_id   = apple_subscription_offer_code.schools.id
  custom_code     = "PILOTSCHOOLS"
  number_of_codes = 100
  active          = false
}
