terraform {
  required_version = ">= 1.3"

  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}

provider "apple" {
  # Credentials come from the environment:
  #   APPLE_APP_STORE_CONNECT_ISSUER_ID
  #   APPLE_APP_STORE_CONNECT_API_KEY
  #   APPLE_APP_STORE_CONNECT_PRIVATE_KEY
}

# The app record itself is never created here: Apple's API cannot create one.
# Create the app on the App Store Connect website, then point this module at it.
data "apple_apps" "this" {
  bundle_id = var.bundle_id
}

locals {
  app_id = data.apple_apps.this.apps[0].id

  # Apple splits localized metadata across two lifetimes, and a flat metadata
  # file of the kind EAS and fastlane use papers over the split. Keeping one
  # map per locale and fanning it out to both resources is the closest a
  # configuration can get to the flat shape without pretending the two records
  # are one.
  #
  #   app_info_localization  - name, subtitle, privacy policy. Survives releases.
  #   version_localization   - description, keywords, promo text, release notes.
  #                            Replaced with the version.
  locales = var.locales
}

# --- The app record: content rights -----------------------------------------

# App Store Connect blocks a submission until Content Rights Information is
# answered: "You must set up Content Rights Information in App Information".
# This is the only thing the API offers for it.
resource "apple_app_settings" "this" {
  app_id = local.app_id

  content_rights_declaration = var.content_rights_declaration
  primary_locale             = var.primary_locale
}

# --- Categories --------------------------------------------------------------

# Categories hang off the AppInfo record, which Apple freezes once a version is
# in review. List the valid constants with the apple_app_categories data source.
resource "apple_app_info" "this" {
  app_id = local.app_id

  primary_category   = var.primary_category
  secondary_category = var.secondary_category
}

# --- Age rating ---------------------------------------------------------------

# Apple computes the rating from these answers. An unset answer is not NONE: it
# leaves Apple's existing value alone, which for a new app means unanswered.
resource "apple_app_age_rating_declaration" "this" {
  app_id = local.app_id

  alcohol_tobacco_or_drug_use_or_references        = var.advisory.alcohol_tobacco_or_drug_use_or_references
  contests                                         = var.advisory.contests
  gambling_simulated                               = var.advisory.gambling_simulated
  guns_or_other_weapons                            = var.advisory.guns_or_other_weapons
  horror_or_fear_themes                            = var.advisory.horror_or_fear_themes
  mature_or_suggestive_themes                      = var.advisory.mature_or_suggestive_themes
  medical_or_treatment_information                 = var.advisory.medical_or_treatment_information
  profanity_or_crude_humor                         = var.advisory.profanity_or_crude_humor
  sexual_content_graphic_and_nudity                = var.advisory.sexual_content_graphic_and_nudity
  sexual_content_or_nudity                         = var.advisory.sexual_content_or_nudity
  violence_cartoon_or_fantasy                      = var.advisory.violence_cartoon_or_fantasy
  violence_realistic                               = var.advisory.violence_realistic
  violence_realistic_prolonged_graphic_or_sadistic = var.advisory.violence_realistic_prolonged_graphic_or_sadistic

  advertising                 = var.advisory.advertising
  age_assurance               = var.advisory.age_assurance
  gambling                    = var.advisory.gambling
  health_or_wellness_topics   = var.advisory.health_or_wellness_topics
  loot_box                    = var.advisory.loot_box
  messaging_and_chat          = var.advisory.messaging_and_chat
  parental_controls           = var.advisory.parental_controls
  social_media                = var.advisory.social_media
  social_media_age_restricted = var.advisory.social_media_age_restricted
  unrestricted_web_access     = var.advisory.unrestricted_web_access
  user_generated_content      = var.advisory.user_generated_content

  age_rating_override_v2    = var.advisory.age_rating_override_v2
  korea_age_rating_override = var.advisory.korea_age_rating_override
}

# --- The app's own name and subtitle, per language ---------------------------

resource "apple_app_info_localization" "this" {
  for_each = local.locales

  app_id = local.app_id
  locale = each.key

  name               = each.value.title
  subtitle           = each.value.subtitle
  privacy_policy_url = each.value.privacy_policy_url
}

# --- The release --------------------------------------------------------------

# An app holds one editable version per platform at a time. If one already
# exists, import it rather than letting this resource try to create a second:
#
#   terraform import apple_app_store_version.this <app_id>/IOS/1.0
resource "apple_app_store_version" "this" {
  app_id         = local.app_id
  platform       = var.platform
  version_string = var.version_string

  copyright = var.copyright

  # The build is named the way a pipeline names it, by build number.
  # pre_release_version defaults to version_string, which is the train App Store
  # Connect offers builds from anyway, so it is left unset.
  build_number = var.build_number

  # "automatic release" is AFTER_APPROVAL; holding it back is MANUAL.
  release_type = var.automatic_release ? "AFTER_APPROVAL" : "MANUAL"
}

resource "apple_app_store_version_localization" "this" {
  for_each = local.locales

  app_store_version_id = apple_app_store_version.this.id
  locale               = each.key

  description      = each.value.description
  keywords         = each.value.keywords
  promotional_text = each.value.promo_text
  marketing_url    = each.value.marketing_url
  support_url      = each.value.support_url

  # Apple rejects release notes on an app's first version and requires them on
  # every version after, so this is null until there is something to say.
  whats_new = each.value.whats_new
}

resource "apple_app_store_review_detail" "this" {
  app_store_version_id = apple_app_store_version.this.id

  contact_first_name = var.review.first_name
  contact_last_name  = var.review.last_name
  contact_email      = var.review.email
  contact_phone      = var.review.phone

  demo_account_required = var.review.demo_required
  demo_account_name     = var.review.demo_account_name
  demo_account_password = var.review.demo_account_password

  notes = var.review.notes
}

# --- Pricing and availability --------------------------------------------------

# A price is never a number: it references one of Apple's price points. A free
# app is the price point whose customer price is 0.
data "apple_app_price_points" "base" {
  app_id         = local.app_id
  territories    = [var.base_territory]
  customer_price = var.customer_price
}

# App Store Connect blocks a submission until a price is chosen: "You must
# choose a price tier in Pricing". Apple retired tiers; this is the replacement.
resource "apple_app_price_schedule" "this" {
  app_id         = local.app_id
  base_territory = var.base_territory

  prices = [
    {
      price_point_id = data.apple_app_price_points.base.price_points[0].id
    },
  ]
}

data "apple_territories" "all" {}

# available_in_new_territories only covers storefronts Apple opens later, so
# selling everywhere means naming everywhere today.
resource "apple_app_availability" "this" {
  app_id                       = local.app_id
  available_in_new_territories = true

  territories = [
    for code in(length(var.territories) > 0 ? var.territories : data.apple_territories.all.ids) :
    { territory = code }
  ]
}
