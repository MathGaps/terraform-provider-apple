## 0.4.2 (October 8, 2026)

MathGaps's first release: upstream
[0.4.1](https://github.com/AhmedOsman00/terraform-provider-apple/releases/tag/v0.4.1),
unchanged, published to the OpenTofu Registry as `mathgaps/apple` (FORK.md).

NOTES:

* The source address is `mathgaps/apple`.
* The releases are signed with MathGaps's key, not upstream's.

## 0.4.1 (September 23, 2026)

BUG FIXES:

* **`apple_beta_tester` no longer fails every apply with "The parameter
  'filter[email]' can not be used with this request".** Apple accepts
  `filter[email]` on the account-wide `/v1/betaTesters` and refuses it with a
  400 on the group's own `/v1/betaGroups/{id}/betaTesters`, which is the
  collection the resource has to ask: the question is membership, not
  existence. Every create and every read sent it, so no tester could be added
  to a group at all. The provider now walks the group's collection and matches
  the address in memory — which it already did, case-insensitively; the filter
  was only ever a narrowing. For a large external group this costs pages rather
  than one request, and Apple offers no server-side membership lookup here.

## 0.4.0 (September 22, 2026)

FEATURES:

* **`apple_user`, `apple_user_invitation` and `apple_users`.** Manages who is on
  your App Store Connect team and what they can do — the Users and Access page,
  as configuration.

  ```terraform
  resource "apple_user_invitation" "new_hire" {
    email      = "ada@example.com"
    first_name = "Ada"
    last_name  = "Lovelace"
    roles      = ["DEVELOPER"]

    provisioning_allowed = true
  }

  resource "apple_user" "release_manager" {
    username = "grace@example.com"
    roles    = ["APP_MANAGER", "ACCESS_TO_REPORTS"]
  }
  ```

  !> **Applying an invitation sends a real email**, and **destroying an
  `apple_user` removes that person from the team** — Apple revokes their access
  to every app, and the API cannot put it back: they have to be invited again
  and accept again.

  **These are two Apple records with two lifetimes, and the split is not
  optional.** A `UserInvitation` is an offer; Apple destroys it the moment
  somebody accepts and issues a `User` in its place, with a different
  identifier. So an invitation that stops being readable has been accepted,
  cancelled, or has lapsed, and the 404 alone cannot say which. `Read` asks the
  team's member list before it drops anything: an accepted invitation sets the
  computed `accepted` and `user_id` and **stays in state** rather than being
  planned for recreation, which Apple would refuse for somebody already on the
  team. One that was cancelled or expired leaves state and is sent again on the
  next apply.

  **`apple_user` adopts; it cannot create.** Apple publishes no endpoint that
  adds a member directly, so pointing it at an address that is not already on
  the team is an error naming `apple_user_invitation`. It is the resource that
  changes roles afterwards: Apple has no `PATCH` for an invitation, so every
  attribute there forces replacement, and replacing an accepted one is not a
  thing that can happen.

  `visible_apps` is read back **only when the configuration sets it**. Apple
  answers the same collection with every app on the team for a member who can
  see all of them, so adopting it unasked would write your whole catalogue into
  state — the rule `apple_app_store_version` follows for a build it does not
  manage.

  `ACCOUNT_HOLDER` is rejected at plan time. Apple reports it for the person who
  owns the membership and refuses every request that grants it, so a
  configuration carrying it could never apply. The account holder cannot be
  managed by this provider at all.

  Both resources import by the address as readily as by Apple's ID — `/v1/users`
  has no single-record read, so the provider scans the collection either way.
  Managing the team needs an App Store Connect API key with the **Admin** role;
  an App Manager key can read the collection and not write to it.

* **`apple_beta_tester`.** Puts a tester in a TestFlight group — the membership
  0.3.0 said was not modelled yet, and the last thing an `apple_beta_group`
  needed before it distributed anything.

  ```terraform
  resource "apple_beta_tester" "early_access" {
    for_each = toset(["ada@example.com", "grace@example.com"])

    group_id = apple_beta_group.early_access.id
    email    = each.value
  }
  ```

  !> **Applying this sends a real TestFlight invitation** to every address it
  names. Point it only at people you are entitled to invite.

  **A tester record is the account's, not the group's.** Apple holds one record
  per email address across every app and group, so putting the same person in
  three groups is three resources carrying the same `id`, and whichever applies
  first is the one that creates the record — the others link it. Nothing
  connects those resources, so Terraform applies them at once; the provider
  retries a create Apple refuses as a duplicate as the link it would have made a
  moment later, which is what saves every such configuration a `depends_on`.

  **Destroying one removes the membership, not the person.** Apple's
  `DELETE /v1/betaTesters/{id}` would take the tester out of every app in the
  account, so the provider withdraws the group linkage instead and leaves the
  record, and every other group, alone.

  Apple publishes no update for a beta tester, so every attribute forces
  replacement. `first_name` and `last_name` are sent only when the provider
  creates the record: point one at an address your account already holds and the
  provider warns that Apple kept the name it has, rather than writing a name
  into state that Apple never applied.

  Import is `<group_id>/<email>`, with no bare-ID form — a tester's Apple ID
  names the person rather than the membership, and the same ID belongs to every
  group that person is in.

  Assigning individual builds to a group is still not modelled;
  `has_access_to_all_builds` on `apple_beta_group` remains the way to give one
  every build.

* **`apple_subscription_price_schedule`.** Prices an auto-renewable subscription
  in every territory in a single request, instead of one resource and one
  `POST /v1/subscriptionPrices` per storefront.

  ```terraform
  data "apple_subscription_price_point_equalizations" "annual" {
    price_point_id = data.apple_subscription_price_points.base.price_points[0].id
  }

  resource "apple_subscription_price_schedule" "annual" {
    subscription_id = apple_subscription.pro_annual.id

    prices = [
      for territory, price_point in data.apple_subscription_price_point_equalizations.annual.price_point_ids : {
        territory_id   = territory
        price_point_id = price_point
      }
    ]

    depends_on = [apple_subscription_availability.pro_annual]
  }
  ```

  **The bulk form is not on `subscriptionPrices`.** That endpoint really does
  take one price point in one territory, which is why Apple's own guidance tells
  callers to automate the loop. The bulk write hangs off the subscription
  instead: `SubscriptionUpdateRequest` carries a `prices` relationship and an
  `included` member accepting `SubscriptionPriceInlineCreate`, so the whole set
  travels inline in one `PATCH /v1/subscriptions/{id}` — the same JSON:API shape
  `apple_in_app_purchase_price_schedule` and `apple_app_price_schedule` already
  used.

  Fanning out across the App Store's 175 storefronts was previously 175
  resources and 175 calls on apply, plus a refresh that listed the
  subscription's whole price collection once *per resource*, because Apple
  publishes no `GET` for a single price. It is now one call and one listing.

  `apple_subscription_price` is unchanged and still right for a single territory
  or a one-off scheduled change. Do not point both at the same subscription:
  every write of the schedule replaces the subscription's manual price set.

  The same prerequisites apply as before — an `apple_subscription_availability`
  has to exist first, and Apple's refusal names neither it nor a territory — and
  Apple additionally refuses the write while the subscription is in review.

## 0.3.0 (September 21, 2026)

FEATURES:

* **TestFlight.** Four resources that cover what `fastlane pilot` does, closing
  the last gap between this provider and the fastlane tools it replaces:
  `apple_beta_group` (a tester group, internal or external),
  `apple_beta_app_localization` (the app's TestFlight page text),
  `apple_beta_build_localization` (one build's "What to Test" note) and
  `apple_beta_app_review_detail` (what Apple's beta reviewers are told).

  ```terraform
  resource "apple_beta_group" "qa" {
    app_id                   = data.apple_apps.this.apps[0].id
    name                     = "QA"
    is_internal_group        = true
    has_access_to_all_builds = true
  }

  resource "apple_beta_build_localization" "whats_new" {
    app_id              = data.apple_apps.this.apps[0].id
    build_number        = "42"
    pre_release_version = "1.2.0"
    locale              = "en-US"
    whats_new           = "Shared budgets, and a rewritten sync engine."
  }
  ```

  **Internal and external groups are not one flag with a different label.**
  `is_internal_group` is absent from Apple's update request, so it is fixed for
  the life of the group, and the two kinds accept disjoint attribute sets: only
  an external group can carry a public link, and the provider rejects
  `public_link_enabled` on an internal one at plan time rather than letting
  Apple refuse it with a message that names the attribute and not the reason.
  `has_access_to_all_builds` is fixed the same way.

  TestFlight metadata splits across two lifetimes exactly as App Store metadata
  does. What describes the app — the description testers read, the feedback
  address, the marketing and privacy links — is `apple_beta_app_localization`
  and survives every build. What describes one build is
  `apple_beta_build_localization` and is replaced with it. That resource names
  its build by `build_number` and `pre_release_version` rather than by Apple's
  opaque ID, the same way `apple_app_store_version` does, and resolves the ID
  itself.

  `apple_beta_app_review_detail` **adopts on create**: Apple publishes no
  `POST` for the record, which comes into existence with the app, so `Create` is
  a `PATCH` and `destroy` drops it from state and warns — the shape
  `apple_app_settings` and `apple_app_info` are already in. It is a different
  record from `apple_app_store_review_detail` and does not substitute for it:
  this one is per app and gates external TestFlight distribution, that one is
  per version and gates the App Store listing.

  Three things are deliberately **not** here. Tester membership and assigning
  builds to a group are real state and are simply not modelled yet; a group
  created here starts empty, and `has_access_to_all_builds` is the way to give
  one every build without them. Submitting a build for beta review is not
  modelled at all, for the reason App Store submission is not: it is an event
  rather than a state, and a resource for it would cancel a live review on
  destroy and resubmit on apply.

* **`apple_subscription_price_point_equalizations` prices a subscription in
  every territory from one base price.** Apple's
  `GET /v1/subscriptionPricePoints/{id}/equalizations` returns the price point
  it considers equivalent to a base point in each other territory — the mapping
  App Store Connect's own price matrix is built from — and the provider now
  reads it.

  `apple_subscription_price_points` answers *what may this subscription cost in
  these territories*, which left a configuration pricing every storefront to
  decide each one by hand: there is no single `customer_price` to filter on,
  because `9.99` in the United States is neither `9.99` nor a round number
  anywhere else. Pick one base price point, pass its ID to the new data source,
  and `price_point_ids` — a territory code to price point ID map, the same shape
  as `ids` on `apple_territories` — drives a `for_each` over
  `apple_subscription_price`:

  ```terraform
  data "apple_subscription_price_point_equalizations" "base" {
    price_point_id = data.apple_subscription_price_points.base.price_points[0].id
  }

  resource "apple_subscription_price" "plan" {
    for_each = data.apple_subscription_price_point_equalizations.base.price_point_ids

    subscription_id = apple_subscription.pro_monthly.id
    price_point_id  = each.value
    territory_id    = each.key

    depends_on = [apple_subscription_availability.pro_monthly]
  }
  ```

  `territories` narrows the read server-side; unlike the catalogue read, leaving
  it unset is the ordinary case, because the endpoint returns one record per
  territory rather than tens of thousands. `apple_subscription_availability`
  must still cover every territory the fan-out prices — Apple refuses a price in
  a territory a subscription is not available in, and names neither in the
  error — so the `depends_on` above is not optional. `examples/resources/apple_subscription_price`
  shows the whole shape end to end.

* **`apple_app_store_version` attaches a build, named the way a pipeline names
  it.** Three new attributes: `build_number` (`CFBundleVersion`, which CI knows
  as `CURRENT_PROJECT_VERSION`), `pre_release_version`
  (`CFBundleShortVersionString` / `MARKETING_VERSION`, defaulting to
  `version_string`) and the computed `build_id`.

  ```terraform
  resource "apple_app_store_version" "this" {
    app_id         = data.apple_apps.this.apps[0].id
    platform       = "IOS"
    version_string = "1.2.0"

    build_number = "42" # pre_release_version defaults to version_string
  }
  ```

  Apple's linkage endpoint takes an opaque build ID that nobody has, so the
  provider resolves the two numbers to it rather than asking for it — which is
  one request, because `/v1/builds` filters on `filter[version]` and
  `filter[preReleaseVersion.version]` server-side. Removing `build_number` from
  a configuration that had it detaches the build; leaving it unset leaves
  whatever is attached alone, so a build attached by hand or by a separate
  pipeline is not torn off by the next apply.

  This provider still does not **upload** builds — that stays with Xcode,
  Transporter and fastlane, and Apple publishes no endpoint for it. A build is
  `PROCESSING` for five to thirty minutes after the upload finishes and Apple
  refuses to attach one until it is `VALID`; the provider reports that state
  rather than blocking the apply on it, so a pipeline that uploads and applies
  in one run should wait in between. Export compliance is unaffected:
  `usesNonExemptEncryption` lives on the build, and a build without it parks the
  version in `WAITING_FOR_EXPORT_COMPLIANCE` however complete the metadata is.

  `examples/app-listing` takes the build number as a variable, and its
  `remaining_manual_steps` output drops attaching the build while naming the
  upload and export compliance explicitly.

DOCUMENTATION:

* Generated reference pages now cover all thirty resources and eighteen data
  sources.
* `docs/guides/getting-started.md` lists the import ID of every TestFlight
  resource and explains why `apple_beta_build_localization` is the one resource
  in the provider with no bare-ID import form: it names its build by
  `build_number` and `pre_release_version`, which cannot be recovered from
  Apple's opaque build ID without two further requests — and whoever is
  importing already has both.
* `docs/guides/code-signing.md` notes that TestFlight is now covered by the
  provider even though `examples/signing` does not declare it, the way it does
  not declare an app listing.

## 0.2.1 (September 17, 2026)

BUG FIXES:

* **`apple_app_info` no longer fails an apply with "Provider produced
  inconsistent result after apply".** Apple's `PATCH /v1/appInfos/{id}` reports
  the six category relationships as links alone — it fills in a linkage only for
  an `include`, which the modify endpoint does not accept — so reading the
  categories off that response wrote null into state for every category the
  apply had just set. The resource now applies the attributes from the `PATCH`
  response and re-reads the app info for the categories. If that read fails the
  apply keeps the configured categories and warns, rather than losing the values
  it wrote.

## 0.2.0 (September 16, 2026)

FEATURES:

* **App Store listing metadata.** Nine resources and three data sources that
  manage what the App Store shows about an app: `apple_app_settings` (content
  rights, primary language, subscription notification URLs), `apple_app_info`
  (categories), `apple_app_info_localization` (name, subtitle, privacy policy
  link), `apple_app_age_rating_declaration` (the content questionnaire),
  `apple_app_store_version` (version string, copyright, release type),
  `apple_app_store_version_localization` (description, keywords, promotional
  text, release notes), `apple_app_store_review_detail` (what App Review is
  told), `apple_app_price_schedule` (what the app costs) and
  `apple_app_availability` (the storefronts it sells in), alongside
  `apple_app_categories`, `apple_app_price_points` and
  `apple_app_store_versions`.

  Together these cover three of the four items App Store Connect blocks a
  submission on. Content Rights Information is `content_rights_declaration` on
  `apple_app_settings`; the price tier is `apple_app_price_schedule`, Apple
  having retired tiers in favour of price points. **App Privacy is the fourth,
  and it has no API** — Apple's published App Store Connect API contains no
  `appDataUsages` resource, so the data-collection questionnaire has to be
  answered once on the website. It is declared per app rather than per version,
  so it does not recur with each release.

* `examples/app-listing` is a runnable module that manages a whole listing from
  one `terraform.tfvars`, the way an EAS `store.config.json` or a fastlane
  `Deliverfile` does. Its `remaining_manual_steps` output names what no
  configuration can do: App Privacy, screenshots, the build upload, and
  submitting for review.

NOTES:

* **Localized metadata is split across two resources, because Apple splits it
  across two records.** The app's name, subtitle and privacy policy link live on
  an `AppInfo` and survive every release; the description, keywords,
  promotional text and release notes live on an `AppStoreVersion` and are
  replaced with it. A flat metadata file hides that; Terraform cannot, since the
  two have different lifecycles. `examples/app-listing` keeps one map and fans
  it out to both.

* **`apple_app_settings`, `apple_app_info` and
  `apple_app_age_rating_declaration` adopt records Apple already created.** Apple
  makes them alongside the app and publishes no `POST` for any of them, so these
  resources patch on create and `terraform destroy` drops them from state with a
  warning rather than deleting anything. The same is true of
  `apple_app_price_schedule` and `apple_app_availability`, which Apple replaces
  wholesale with a `POST` and never deletes.

* **Listing metadata can only be written while a version is being prepared.**
  Apple freezes an app info once it is in review or distributed and publishes no
  way to create a fresh one, so the categories, the localized name and the age
  rating cannot change until the current review finishes. The provider resolves
  the editable record itself and says so plainly when there is none.

* **The age rating questionnaire has changed.** The provider models Apple's
  current form, which adds `age_assurance`, `loot_box`, `messaging_and_chat`,
  `parental_controls`, `social_media`, `social_media_age_restricted`,
  `user_generated_content`, `advertising`, `health_or_wellness_topics` and
  `guns_or_other_weapons`, and replaces `age_rating_override` with
  `age_rating_override_v2` (in which `SEVENTEEN_PLUS` became `EIGHTEEN_PLUS`).
  Both override attributes and both frequency vocabularies are accepted, because
  an existing declaration may hold either. An omitted answer is **not** `NONE`:
  Apple leaves it as it was, which for a new app means unanswered.

* **`keywords` is a list in Terraform and one 100-character comma-separated
  string at Apple.** The provider joins with no space after the comma, because a
  space is a character and every one comes out of the hundred. The cap applies
  to the joined string, so it is checked at plan time rather than per keyword.

* Two acceptance tests — `TestAccAppPriceScheduleResource_basic` and
  `TestAccAppAvailabilityResource_basic` — skip unless
  `APPLE_TEST_ALLOW_APP_PRICING` is set, because they change what an app costs
  and where it sells and Apple publishes no `DELETE` for either record. The rest
  of the app listing tests edit the `APPLE_TEST_APP_ID` app's own metadata in
  place; point it at a scratch app. `ACCEPTANCE_TESTING.md` has the detail.

## 0.1.0 (September 9, 2026)

Initial release.

NOTES:

* The provider is published on the Terraform Registry as `ahmedosman00/apple`;
  `terraform init` installs it, and the Go module path is
  `github.com/AhmedOsman00/terraform-provider-apple`. Earlier working copies
  served the development address `aostudio.com/aostudio/apple` and had to be
  built locally.

FEATURES:

* **Developer Portal:** `apple_bundle_id`, `apple_bundle_id_capability`,
  `apple_certificate`, `apple_device`, `apple_merchant_id`,
  `apple_pass_type_id` and `apple_profile`, each with a plural data source that
  lists the collection with filtering, `sort_by`, `sort_order` and `limit`.
  Together these cover what a build pipeline needs from the portal: App IDs and
  their capabilities, signing certificates, registered devices, Apple Pay
  Merchant IDs, Apple Wallet Pass Type IDs, and provisioning profiles. Two
  destroy behaviours are worth knowing before the first apply: destroying an
  `apple_certificate` revokes it at Apple, and every build already signed with
  it stops verifying; and Apple's API cannot delete a device, so removing an
  `apple_device` disables it and drops it from state with a warning.
* `apple_profile.profile_type` is the required argument that selects the
  distribution method — development, ad hoc, App Store, or in-house — and
  `platform` is computed from it rather than configured. Apple's
  `POST /v1/profiles` takes `profileType`, which encodes the platform, and does
  not accept `platform` at all. `profile_content` is marked sensitive, matching
  `certificate_content` on `apple_certificate`, so outputs deriving values from
  it must be declared `sensitive = true`.
* **App Store Connect — auto-renewable subscriptions:**
  `apple_subscription_group`, `apple_subscription`,
  `apple_subscription_localization`, `apple_subscription_price` and
  `apple_subscription_availability`, with `apple_subscription_groups`,
  `apple_subscriptions` and `apple_subscription_price_points` data sources.
  These behave differently from Developer Portal resources: they hang off an
  app record, and Apple publishes no top-level collection for any of them, so
  every listing takes a required scope argument and several import forms are
  composite. Two ordering rules are worth knowing before the first apply. A
  subscription needs an `apple_subscription_availability` before it can be
  priced — Apple rejects a price without one and says only that "an error
  occurred while processing the pricing information", so declare `depends_on`
  from the price to the availability, since nothing else connects them. And
  like the in-app purchase availability, the record is a singular one Apple
  replaces with a `POST` and publishes no `DELETE` for: it updates in place and
  cannot be destroyed. Destroying an `apple_subscription_price` that is already
  in effect likewise warns and drops state rather than deleting: Apple removes
  only price changes scheduled for the future, and a live price is superseded by
  a later one rather than withdrawn. Deleting the subscription removes both.
* **App Store Connect — one-time in-app purchases:** `apple_in_app_purchase`,
  `apple_in_app_purchase_localization`, `apple_in_app_purchase_price_schedule`
  and `apple_in_app_purchase_availability`, with `apple_in_app_purchases` and
  `apple_in_app_purchase_price_points` data sources. These are the consumables,
  non-consumables and non-renewing subscriptions of the App Store — a different
  Apple resource from auto-renewable subscriptions, sharing nothing with them,
  price points included. Three shapes are worth knowing before you plan:
  `app_id` is write-once and unreadable, because Apple's in-app purchase
  resource has no app relationship at all, so import takes
  `<app_id>/<in_app_purchase_id>`; localizations sit behind an in-app purchase
  version, which the provider resolves and reports as `version_id`; and the
  price schedule and availability are singular records Apple replaces with a
  `POST` and publishes no `DELETE` for, so they update in place and cannot be
  destroyed. Destroying the *only* localization of a purchase likewise warns and
  drops state rather than deleting: Apple requires every version to keep one.
  Deleting the `apple_in_app_purchase` removes it for real.
* `apple_apps` data source. There is deliberately no `apple_app` resource —
  Apple's documentation says to create new apps on the App Store Connect
  website and publishes no endpoint to create or delete one — but a
  subscription group needs the app's ID, so it has to be readable.
* examples/signing: a runnable module replacing `fastlane match`, covering the App
  ID, capabilities, devices, signing certificates, and development/Ad Hoc/App Store
  profiles. The signing key is generated on your machine and never reaches
  Terraform: the module takes certificate signing requests through
  `var.csr_contents` and exports the issued certificates and generated profiles
  as plain values. A CSR, a certificate and a provisioning profile are all public
  documents, so nothing in the state or the outputs is secret, and installing the
  result is a `.p12` built locally from the certificate and the key you kept.
* examples/signing: `var.adopt_certificate_serials` adopts a certificate
  `fastlane match` already issued, reading it through the `apple_certificates`
  data source rather than reissuing — which would revoke the original and break
  every build already signed with it. An adopted role needs no CSR; keep using
  the key match holds for it.

DOCUMENTATION:

* Generated reference pages for all sixteen resources and thirteen data
  sources, built by `tfplugindocs` from the schemas and the matching
  directories under `examples/`.
* Two hand-written guides: `docs/guides/getting-started.md` (creating App Store
  Connect credentials, installing the provider, a first configuration, and
  importing existing portal resources) and `docs/guides/code-signing.md` (the
  `fastlane match` replacement, generating a CSR and installing the issued
  certificate by hand, what actually needs protecting once the key never enters
  Terraform, and migrating off match). Guides live in `templates/guides/` and
  render into `docs/guides/`.
