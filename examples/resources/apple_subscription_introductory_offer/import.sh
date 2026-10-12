# The import ID is "<subscription_id>/<offer_id>", where the offer ID is the
# record of any one territory -- apple_subscription_introductory_offers lists
# them. Every record on the subscription with the same terms is adopted with
# it, which is what one offer across many territories looks like at Apple.
terraform import apple_subscription_introductory_offer.pro_monthly_trial 6740021496/9a1b2c3d-4e5f-6789-abcd-ef0123456789
