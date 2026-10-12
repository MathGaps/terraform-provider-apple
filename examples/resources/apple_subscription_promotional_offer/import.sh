# By Apple's offer ID. The subscription is read back from the offer.
terraform import apple_subscription_promotional_offer.win_back 6741234567

# Or by "<subscription_id>/<offer_code>", the form an offer made in App Store
# Connect is known by: the offer code is what the app already holds.
terraform import apple_subscription_promotional_offer.win_back 6740021496/winback_half_3m
