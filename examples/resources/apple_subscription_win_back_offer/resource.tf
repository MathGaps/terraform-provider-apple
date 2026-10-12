data "apple_apps" "example" {
  bundle_id = "com.example.app"
}

data "apple_subscription_groups" "premium" {
  app_id = data.apple_apps.example.apps[0].id
}

# A win-back offer needs a subscription App Review has already approved, so it
# is usually read rather than created alongside the offer.
data "apple_subscriptions" "pro_monthly" {
  group_id           = data.apple_subscription_groups.premium.subscription_groups[0].id
  product_id_pattern = "^com\\.example\\.app\\.pro\\.monthly$"
}

# The offer price in one storefront, fanned out to every other one.
data "apple_subscription_price_points" "offer_base" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
  territories     = ["USA"]
  customer_price  = "4.99"
}

data "apple_subscription_price_point_equalizations" "offer" {
  price_point_id = data.apple_subscription_price_points.offer_base.price_points[0].id
}

# Half price for three months, for customers who paid for at least six months
# and lapsed between two months and a year ago.
resource "apple_subscription_win_back_offer" "half_price" {
  subscription_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
  reference_name  = "Pro Monthly win-back 2027"
  # Treat an offer_id as used for good: a replacement offer needs a new one.
  offer_id     = "pro_monthly_winback_2027"
  duration     = "ONE_MONTH"
  offer_mode   = "PAY_AS_YOU_GO"
  period_count = 3

  customer_eligibility_paid_subscription_duration_in_months = 6
  customer_eligibility_time_since_last_subscribed_in_months = { minimum = 2, maximum = 12 }
  customer_eligibility_wait_between_offers_in_months        = 6

  start_date       = "2027-01-15"
  end_date         = "2027-03-31"
  priority         = "HIGH"
  promotion_intent = "NOT_PROMOTED"

  # Prices cannot change after creation: editing this set replaces the offer.
  prices = concat(
    [{
      territory_id   = "USA"
      price_point_id = data.apple_subscription_price_points.offer_base.price_points[0].id
    }],
    [
      for territory, price_point in data.apple_subscription_price_point_equalizations.offer.price_point_ids : {
        territory_id   = territory
        price_point_id = price_point
      } if territory != "USA"
    ],
  )
}
