# Every introductory offer record on a subscription, one per territory --
# including ones made in App Store Connect, whose IDs are what
# apple_subscription_introductory_offer imports from.
data "apple_subscription_introductory_offers" "pro_monthly" {
  subscription_id = "6740021496"
}

# Narrowed by territory (applied by Apple) and by mode.
data "apple_subscription_introductory_offers" "trials_in_europe" {
  subscription_id = "6740021496"
  territories     = ["GBR", "DEU", "FRA"]
  offer_mode      = "FREE_TRIAL"
}

output "offer_ids_by_territory" {
  value = {
    for offer in data.apple_subscription_introductory_offers.pro_monthly.introductory_offers :
    offer.territory_id => offer.id...
  }
}
