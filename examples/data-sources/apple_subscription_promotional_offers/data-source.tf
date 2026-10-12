data "apple_apps" "example" {
  bundle_id = "com.example.app"
}

data "apple_subscription_groups" "premium" {
  app_id = data.apple_apps.example.apps[0].id
}

data "apple_subscriptions" "pro_monthly" {
  group_id           = data.apple_subscription_groups.premium.subscription_groups[0].id
  product_id_pattern = "^com\\.example\\.app\\.pro\\.monthly$"
}

# Every promotional offer on the subscription, including ones made by hand in
# App Store Connect.
data "apple_subscription_promotional_offers" "pro_monthly" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
}

# One offer, found by the code the app requests it with.
data "apple_subscription_promotional_offers" "win_back" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
  offer_code      = "winback_half_3m"
}

output "offer_codes" {
  value = [for offer in data.apple_subscription_promotional_offers.pro_monthly.promotional_offers : offer.offer_code]
}
