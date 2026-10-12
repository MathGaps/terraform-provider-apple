data "apple_apps" "example" {
  bundle_id = "com.example.app"
}

resource "apple_subscription_group" "premium" {
  app_id         = data.apple_apps.example.apps[0].id
  reference_name = "Premium"
}

resource "apple_subscription" "pro_annual" {
  group_id            = apple_subscription_group.premium.id
  product_id          = "com.example.app.pro.annual"
  name                = "Pro Annual"
  subscription_period = "ONE_YEAR"
}

# An offer code targets one subscription -- one tier, one billing period --
# the way a coupon restricted to one price does.
#
# A free trial has no price, only the territories it is offered in.
resource "apple_subscription_offer_code" "schools_free_month" {
  subscription_id        = apple_subscription.pro_annual.id
  name                   = "Schools free month 2026"
  customer_eligibilities = ["NEW", "EXPIRED"]
  offer_eligibility      = "STACK_WITH_INTRO_OFFERS"
  duration               = "ONE_MONTH"
  offer_mode             = "FREE_TRIAL"
  number_of_periods      = 1

  prices = [
    { territory_id = "AUS" },
    { territory_id = "USA" },
  ]
}

# A paid offer names one of the subscription's own price points per
# territory: 49.99 in the USA for the first year, and whatever Apple
# considers equivalent elsewhere.
data "apple_subscription_price_points" "discount_usa" {
  subscription_id = apple_subscription.pro_annual.id
  territories     = ["USA"]
  customer_price  = "49.99"
}

data "apple_subscription_price_point_equalizations" "discount" {
  price_point_id = data.apple_subscription_price_points.discount_usa.price_points[0].id
  territories    = ["AUS", "GBR"]
}

resource "apple_subscription_offer_code" "half_price_first_year" {
  subscription_id        = apple_subscription.pro_annual.id
  name                   = "Half price first year"
  customer_eligibilities = ["NEW"]
  offer_eligibility      = "REPLACE_INTRO_OFFERS"
  duration               = "ONE_YEAR"
  offer_mode             = "PAY_UP_FRONT"
  number_of_periods      = 1

  prices = [
    for territory, price_point in merge(
      { USA = data.apple_subscription_price_points.discount_usa.price_points[0].id },
      data.apple_subscription_price_point_equalizations.discount.price_point_ids,
      ) : {
      territory_id   = territory
      price_point_id = price_point
    }
  ]

  # Every attribute but active is fixed at creation, and a replacement
  # deactivates the old offer -- invalidating codes already handed out. If
  # Apple revises its equalizations, keep the running offer rather than
  # replacing it.
  lifecycle {
    ignore_changes = [prices]
  }
}

# Ending a campaign: active = false deactivates the offer in place, for good.
# Apple cannot reactivate it; setting it back to true plans a new offer.
resource "apple_subscription_offer_code" "spring_2026" {
  subscription_id        = apple_subscription.pro_annual.id
  name                   = "Spring 2026"
  customer_eligibilities = ["EXPIRED"]
  offer_eligibility      = "STACK_WITH_INTRO_OFFERS"
  duration               = "ONE_WEEK"
  offer_mode             = "FREE_TRIAL"
  number_of_periods      = 2
  prices                 = [{ territory_id = "USA" }]
  active                 = false
}
