# Win-back offers import as "<subscription_id>/<offer>", where <offer> is either
# Apple's win-back offer ID or the offer_id.
#
# The subscription ID is required because Apple never reports which
# subscription a win-back offer belongs to.
terraform import apple_subscription_win_back_offer.half_price 6739472901/pro_monthly_winback_2027
