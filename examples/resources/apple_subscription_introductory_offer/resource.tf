# One offer, described once, across a set of territories. Apple keeps a record
# per territory; this resource owns one in each territory it names and creates
# them all in a single request.
#
# The subscription has to be available and priced in every territory first
# (see apple_subscription_availability and apple_subscription_price_schedule).
# Nothing in an offer references either, so when they are managed in the same
# configuration, declare the ordering with depends_on.

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

data "apple_subscriptions" "pro_annual" {
  group_id           = data.apple_subscription_groups.premium.subscription_groups[0].id
  product_id_pattern = "^com\\.example\\.app\\.pro\\.annual$"
}

locals {
  pro_monthly_id = data.apple_subscriptions.pro_monthly.subscriptions[0].id
  pro_annual_id  = data.apple_subscriptions.pro_annual.subscriptions[0].id
}

# A one-week free trial everywhere the App Store sells.
data "apple_territories" "all" {}

resource "apple_subscription_introductory_offer" "pro_monthly_trial" {
  subscription_id = local.pro_monthly_id
  offer_mode      = "FREE_TRIAL"
  duration        = "ONE_WEEK"
  territory_ids   = data.apple_territories.all.ids
}

# Three months at 0.99 for new annual subscribers, priced in every territory
# from one base price point -- the same fan-out the price schedule uses.
data "apple_subscription_price_points" "intro_base" {
  subscription_id = local.pro_annual_id
  territories     = ["USA"]
  customer_price  = "0.99"
}

data "apple_subscription_price_point_equalizations" "intro" {
  price_point_id = data.apple_subscription_price_points.intro_base.price_points[0].id
}

resource "apple_subscription_introductory_offer" "pro_annual_discount" {
  subscription_id   = local.pro_annual_id
  offer_mode        = "PAY_AS_YOU_GO"
  duration          = "ONE_MONTH"
  number_of_periods = 3
  end_date          = "2027-06-30"

  price_point_ids = merge(
    { USA = data.apple_subscription_price_points.intro_base.price_points[0].id },
    data.apple_subscription_price_point_equalizations.intro.price_point_ids,
  )
}
