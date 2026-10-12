# Opt-in, and only in the configuration that hands the codes out: the CSV is
# sensitive in plans but sits in state in plain text, so keep that state in an
# encrypted, access-controlled backend.
data "apple_subscription_offer_code_one_time_use_code_values" "term_1" {
  one_time_use_code_id = "9912345601"
}

# Inspect the document once; split it with csvdecode() if it has a header row,
# or by line if it does not.
locals {
  term_1_lines = compact(split("\n", data.apple_subscription_offer_code_one_time_use_code_values.term_1.csv))
}
