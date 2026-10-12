# Every offer of a subscription, including deactivated ones -- Apple keeps them
# all. The IDs are what terraform import takes for an offer made by hand.
data "apple_subscription_offer_codes" "all" {
  subscription_id = "6740021496"
}

# name_pattern is a regular expression matched as a substring.
data "apple_subscription_offer_codes" "live_school_offers" {
  subscription_id = "6740021496"
  name_pattern    = "(?i)school"
  active          = true
}
