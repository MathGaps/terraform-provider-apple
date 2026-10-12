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

locals {
  pro_monthly_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
}

# The offer price: what 4.99 in the United States is, and what that is worth
# in every other storefront the subscription sells in. The territories have to
# be ones the subscription's apple_subscription_availability covers.
data "apple_subscription_price_points" "offer_base" {
  subscription_id = local.pro_monthly_id
  territories     = ["USA"]
  customer_price  = "4.99"
}

data "apple_subscription_price_point_equalizations" "offer" {
  price_point_id = data.apple_subscription_price_points.offer_base.price_points[0].id
  territories    = ["GBR", "AUS"]
}

locals {
  offer_price_points = merge(
    { USA = data.apple_subscription_price_points.offer_base.price_points[0].id },
    data.apple_subscription_price_point_equalizations.offer.price_point_ids,
  )
}

# Half price for three months, for lapsed monthly subscribers. The app asks the
# App Store for it by offer_code, with a signature its server computes; who is
# eligible is the app's decision. Everything but prices is fixed at creation.
resource "apple_subscription_promotional_offer" "win_back" {
  subscription_id               = local.pro_monthly_id
  name                          = "Win back - half price"
  offer_code                    = "winback_half_3m"
  duration                      = "ONE_MONTH"
  offer_mode                    = "PAY_AS_YOU_GO"
  number_of_periods             = 3
  target_subscription_plan_type = "MONTHLY"

  prices = [
    for territory, price_point in local.offer_price_points : {
      territory_id   = territory
      price_point_id = price_point
    }
  ]
}

# A free trial names its territories and no price point.
resource "apple_subscription_promotional_offer" "free_month" {
  subscription_id   = local.pro_monthly_id
  name              = "Free month"
  offer_code        = "free_month"
  duration          = "ONE_MONTH"
  offer_mode        = "FREE_TRIAL"
  number_of_periods = 1

  prices = [
    for territory in ["USA", "GBR", "AUS"] : { territory_id = territory }
  ]
}
