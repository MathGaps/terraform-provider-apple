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

# Every win-back offer on the plan, including ones made in App Store Connect.
data "apple_subscription_win_back_offers" "pro_monthly" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
}

# One offer by its offer_id, for example to find the import ID of an offer made
# by hand: "<subscription_id>/<offer_id>".
data "apple_subscription_win_back_offers" "half_price" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
  offer_id        = "pro_monthly_winback_2027"
}

output "win_back_offer_ids" {
  value = [for offer in data.apple_subscription_win_back_offers.pro_monthly.win_back_offers : offer.offer_id]
}
