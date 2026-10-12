# Acceptance testing against a real Apple account

Every acceptance test in `internal/provider` talks to the live App Store Connect
API. This file records what each one creates in your Apple Developer account,
where it shows up in the portal, and what it leaves behind.

Read [Devices](#devices--the-only-irreversible-tests) before you export
credentials, and [Team members](#team-members-users-and-access) if you set the
variables that unlock it. Those are the two sections describing changes you
cannot undo — a device slot that never comes back, and a person removed from the
team who has to be invited again.

The credential-free tests (`internal/apple`, the `filters_test.go` files,
`capability_models_test.go`, `renewal_test.go`, `models_test.go`) never reach
Apple and are not covered here.

## Is any of this going to get the account suspended?

No. The suite does none of the things Apple acts on: no credential
brute-forcing, no scraping, no bulk account creation, no circumvention. A full
run is on the order of a few hundred API calls spread over many minutes, well
under the rate ceiling, and `doRequest` reports a 401 rather than retrying, so
there is no retry storm to mistake for an attack.

There is one clause worth knowing about: the device tests register fabricated
UDIDs, and the Developer Program License Agreement frames device registration as
being for devices you own or control. In practice that is a paperwork concern
rather than an enforcement one — but it is the only place the suite touches the
agreement, and the only permanent change it makes.

Run the suite against a dedicated or secondary team where you can. Not because
of suspension, but because device slots and certificate slots are finite and
shared with whatever ships from that team.

## What a clean full run costs

Assuming every test passes and every destroy completes:

| | Count | |
|---|---|---|
| **Permanent** | 2 | Device registrations that can never be removed — one iOS, one Mac. Terraform disables them; they still occupy slots until the membership year renews. |
| **Transient peak** | 2 | Certificates alive at once, at most. Tests run sequentially and each revokes what it issued. |
| **Net zero** | 17 | Resource kinds fully cleaned up: Bundle IDs, capabilities, Merchant IDs, Pass Type IDs, profiles, subscription groups, subscriptions, localizations, prices, promotional offers, in-app purchases, their localizations, price schedules and availability, beta groups, both TestFlight localizations, and win-back offers if `APPLE_TEST_APPROVED_SUBSCRIPTION_ID` is set. |
| **Opt-in residue** | 1 | A beta tester record in the account, if `APPLE_TEST_BETA_TESTER_EMAIL` is set. The membership is removed; the record is not — see [TestFlight](#testflight). |
| **Opt-in removal** | 1 | A team member removed from Users and Access, if `APPLE_TEST_USER_EMAIL` and `APPLE_TEST_ALLOW_USER_REMOVAL` are both set. They have to be invited again — see [Team members](#team-members-users-and-access). |
| **Reserved forever** | ~16 | Subscription and in-app purchase product identifiers, randomised per run, plus one win-back `offer_id` when that test is opted into, and possibly promotional offer codes, whose release Apple does not document. Invisible once deleted, drawn from an unlimited namespace — see [Subscriptions](#subscriptions), [Win-back offers](#win-back-offers) and [In-app purchases](#in-app-purchases). |
| **Touch nothing** | 30 | Tests rejected at plan time or read-only. |

### Certificates come back; devices do not

This is the one distinction that matters when reading the tables below.

For **certificates**, the cap is on how many are active *at once*, not how many
you have ever issued. `Delete` calls `DELETE /v1/certificates/{id}`, which is
Apple's revoke endpoint (`internal/apple/certificates.go`). A revoked
certificate drops off the list immediately and the slot is free that instant.
There is no lifetime counter. `testAccCheckCertificateDestroy` asserts this by
checking the certificate is no longer *listed* rather than GETting it by ID.

For **devices**, the cap is on how many you have ever registered in the
membership year. Apple has no delete at all, so the provider disables instead
(`PATCH … status: DISABLED`). Disabling releases the device from provisioning
profiles but does **not** give the slot back. The count resets only at annual
renewal.

Note also that revoking a certificate invalidates every provisioning profile
built against it — signed ad hoc and enterprise builds already in the field stop
launching, though App Store builds are fine since Apple re-signs those. The
suite only ever revokes certificates it issued itself, so your own certificates
and their profiles are untouched. It is still worth internalising for normal use
of the provider: `terraform destroy` on an `apple_certificate` is a revocation,
not a soft delete.

## Where each thing appears in the portal

Most of it lands under **Certificates, Identifiers & Profiles** on
developer.apple.com. The subscription and in-app purchase tests are the
exception: they work in App Store Connect proper, under the app named by
`APPLE_TEST_APP_ID`. The team membership tests are a second exception, and work
on the team itself rather than on any app. Neither tier creates apps, builds or
submissions.

| Resource | Portal location |
|---|---|
| Bundle IDs | Identifiers → filter "App IDs" |
| Capabilities | Identifiers → open the App ID → Capabilities checklist |
| Merchant IDs | Identifiers → filter "Merchant IDs" |
| Pass Type IDs | Identifiers → filter "Pass Type IDs" |
| Certificates | Certificates (revoked ones drop off the list) |
| Devices | Devices → toggle "Include disabled devices" |
| Profiles | Profiles |
| Subscription groups | App Store Connect → the app → Monetization → Subscriptions |
| Subscriptions | App Store Connect → the app → Monetization → Subscriptions → open the group |
| Localizations and prices | App Store Connect → open the subscription |
| In-app purchases | App Store Connect → the app → Monetization → In-App Purchases |
| Purchase metadata, price and availability | App Store Connect → open the in-app purchase |
| Beta groups | App Store Connect → the app → TestFlight → Testers and Groups |
| TestFlight page text | App Store Connect → the app → TestFlight → Test Information |
| "What to Test" notes | App Store Connect → the app → TestFlight → open the build |
| Beta review information | App Store Connect → the app → TestFlight → Test Information → Beta App Review Information |
| Team members and invitations | App Store Connect → Users and Access → Users |

## Devices — the only irreversible tests

Neither device test declares a `CheckDestroy`, and both use hard-coded UDIDs.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccDeviceResource` | **Permanent** | Registers iOS UDID `00008030-000A4D8E0AB8802E` as "Test Device", imports it, renames it to "Updated Test Device", then disables it. | **Devices** gains a permanent iPhone row named *Updated Test Device*, status *Disabled*. One iOS slot consumed for the membership year. |
| `TestAccDeviceResourceMacOS` | **Permanent** | Registers Mac UDID `550e8400-e29b-41d4-a716-446655440000` as "Test Mac", then disables it. | **Devices** gains a permanent Mac row named *Test Mac*, status *Disabled*. One Mac slot consumed. |
| `TestAccDevicesDataSource` | Read-only | Nothing. Lists every device in the team. | No change. |
| `TestAccDevicesDataSourceFiltered` | Read-only | Nothing. Lists devices, filters to `IOS` in memory. | No change. |
| `TestAccDevicesDataSourceWithLimit` | Read-only | Nothing. Lists devices, takes 5. | No change. |

The two resource tests are effectively one-shot. A second run tries to register
the same UDIDs and the provider maps Apple's 409 to *"Device Already Exists"*,
so expect them to fail on every run after the first unless you re-enable and
reuse the existing entries. Skip them with `-skip 'TestAccDeviceResource'`.

## Certificates

CSRs are generated in Go per test (`testAccCertificateCSR`), so no key material
touches your keychain. A run killed between create and destroy leaves a live
certificate you must revoke by hand.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccCertificateResource_basic` | Slot, returned | Issues 1 × `IOS_DEVELOPMENT`; imports it twice (by serial, by display name); revokes. | One Apple Development certificate mid-run, gone at the end. |
| `TestAccCertificateResource_earlyRenewal` | Slot, returned | Issues 1 × `IOS_DEVELOPMENT`, updates `early_renewal_hours` in place, then sets it to 99999 h — wider than a certificate's one-year life — forcing a reissue. 2 issued in sequence, 2 revoked. | Two Apple Development certificates appear and disappear one after the other, with different serials. |
| `TestAccCertificateResource_requiresReplace` | Slot, returned | Issues `IOS_DEVELOPMENT`, then replaces it with `IOS_DISTRIBUTION`. Both revoked. | One Development then one Distribution certificate. The only certificate test that consumes a *distribution* slot. |
| `TestAccCertificateResource_validation` | Plan-only | Nothing. Bad type, bad CSR and negative hours are all rejected by validators. | No change. |
| `TestAccCertificatesDataSource_basic` | Slot, returned | Issues 1 × `IOS_DEVELOPMENT`, reads it back through four data sources, revokes. | One Apple Development certificate appears and disappears. |
| `TestAccCertificatesDataSource_validation` | Plan-only | Nothing. | No change. |

Across the whole suite (certificate plus profile tests) roughly nine
certificates are issued and revoked, but never more than two are alive at once —
no test calls `t.Parallel()`. If the team is already at its
distribution-certificate cap, the profile tests fail to create rather than
evicting anything.

## Bundle IDs

`testAccCheckBundleIDDestroy` asserts nothing — it is a stub — so a failed
delete would go unnoticed by the test.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccBundleIDResource_basic` | Net zero | `com.test.terraform-example` (iOS), renamed "Test Example" → "Updated Example". | One App ID appears, its name changes, then it is deleted. |
| `TestAccBundleIDResource_platforms` | Net zero | Three subtests: `com.test.platform-ios`, `-mac-os`, `-universal`. | Three App IDs appear and are deleted, one per subtest. |
| `TestAccBundleIDResource_validation` | Plan-only | Nothing — bad identifier, empty name, bad platform all fail validation. | No change. |
| `TestAccBundleIDResource_requiresReplace` | Net zero | `com.test.original`, then replaced by `com.test.changed`. | Two App IDs appear in sequence, both deleted. |
| `TestAccBundleIDResource_disappears` | Net zero | `com.test.terraform-disappear`, deleted at Apple behind Terraform's back to force drift. | One App ID appears and is deleted mid-test; the destroy finds it already gone. |
| `TestAccBundleIDsDataSource_basic` | Net zero | `com.test.datasource`, plus an unfiltered list of every Bundle ID in the team. | One App ID appears and is deleted. |
| `TestAccBundleIDsDataSource_empty` | Read-only | Nothing. Lists every Bundle ID. | No change. |
| `TestAccBundleIDsDataSource_filtering` | Net zero | `com.test.filter.ios` (iOS) and `com.test.filter.macos` (macOS). | Two App IDs appear and are deleted. |
| `TestAccBundleIDsDataSource_sorting` | Net zero | `com.test.sort.aaa` and `com.test.sort.zzz`. | Two App IDs appear and are deleted. |
| `TestAccBundleIDsDataSource_limit` | Read-only | Nothing. Lists Bundle IDs, takes 2. | No change. |
| `TestAccBundleIDsDataSource_validation` | Plan-only | Nothing. | No change. |

`_platforms` derives its fixtures through `testAccBundleIDPlatformFixture`
rather than interpolating the platform constant directly. `BundleIdentifierValidator`
is `^[a-zA-Z0-9.-]+\.[a-zA-Z0-9.-]+$`, so `com.test.MAC_OS` would fail at plan
time, and Apple's App ID name accepts only alphanumerics and spaces, so the name
has to lose the underscore too. Keep both substitutions if you touch that test.

The subtests cover `IOS`, `MAC_OS` and `UNIVERSAL`, which is everything Apple
accepts: `TV_OS` and `WATCH_OS` are rejected with a 409 naming the three valid
values, and tvOS and watchOS App IDs are created as `UNIVERSAL`. Apple then
stores *every* Bundle ID as `UNIVERSAL` whatever was sent, so `platform` records
what was configured and import ignores it.

## Bundle ID capabilities

Each test creates its own parent Bundle ID. Both are deletable, so a completed
run leaves nothing.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccBundleIDCapabilityResource_basic` | Net zero | `com.test.terraform-capability` + Push Notifications; then imports it as `<bundle>/<capability>`. | An App ID appears with *Push Notifications* ticked, then both are deleted. |
| `TestAccBundleIDCapabilityResource_importBareID` | Net zero | Same Bundle ID + Push Notifications, imported by bare capability ID. | Same as above. `bundle_id` is recovered from the capability ID, which Apple composes as `<bundleID>_<TYPE>`, and confirmed against the parent before it is written, so the import is complete rather than partial. |
| `TestAccBundleIDCapabilityResource_settings` | Net zero | Same Bundle ID + Data Protection, flipping the level from *Complete Protection* to *Protected Until First User Auth* in place. | The App ID's *Data Protection* capability shows a changed protection level mid-run. |
| `TestAccBundleIDCapabilityResource_requiresReplace` | Net zero | Same Bundle ID; Push Notifications replaced by HealthKit. | The capability tick moves from *Push Notifications* to *HealthKit*. |
| `TestAccBundleIDCapabilityResource_validation` | Plan-only | Nothing. | No change. |
| `TestAccBundleIDCapabilitiesDataSource_basic` | Net zero | `com.test.terraform-capability-ds` + Push Notifications + HealthKit; reads back via four data sources. | An App ID with two capabilities ticked, then deleted. |
| `TestAccBundleIDCapabilitiesDataSource_validation` | Plan-only | Nothing. | No change. |

Two things about capabilities are worth knowing before reading a failure here.
Apple exposes no `GET` for a single capability — it answers 403 `does not allow
'GET_INSTANCE'` — so every read goes through the parent Bundle ID's collection,
and that collection rejects a `limit` parameter. And Apple applies a capability
write as a read-modify-write over the whole set without locking, so two
concurrent creates on one Bundle ID both return 201 while only one survives; the
provider serializes capability writes per Bundle ID to prevent it.

Apple also enables some capabilities on a new Bundle ID by itself
(`IN_APP_PURCHASE` among them), which is why the data source test asserts a
floor on the count rather than an exact number.

## Subscriptions

**These tests need an app that already exists.** Apple's API cannot create an
App Store Connect app record — its documentation says "Don't use this API to
create new apps; instead, create new apps on the App Store Connect website" —
so every `TestAccSubscription*`, `TestAccApps*` and `TestAccBeta*` test skips itself unless
`APPLE_TEST_APP_ID` names one. Set it to Apple's numeric app ID, the one in the
App Store Connect URL, not the bundle identifier:

```sh
export APPLE_TEST_APP_ID=6448459855
```

Everything these tests create is deletable while it has never been approved, so
a completed run is net zero. One thing is not reversible, and it is worth
understanding before the first run:

**Apple never releases a subscription product identifier.** Not when the
subscription is deleted, not when it was never approved, never. Because of that
these are the only tests in the suite that *randomise* their identifiers rather
than fixing them — a fixed product ID would pass once per account and fail on
every run afterwards, exactly the way the device tests do. The cost is that each
run consumes a handful of identifiers of the form
`com.test.terraform.sub<random>` permanently. They come from an unlimited
namespace and are invisible in the portal once the subscription is deleted, so
this is bookkeeping rather than a real constraint — but it is why the identifier
convention differs here.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccSubscriptionGroupResource_basic` | Net zero | A subscription group under the test app, renamed once, then imported as `<app>/<group>`. | A group appears under *Subscriptions*, its reference name changes, then it is deleted. |
| `TestAccSubscriptionGroupResource_importRejectsBareID` | Net zero | One group; then attempts a bare-ID import and expects it to be refused. | A group appears and is deleted. The failed import changes nothing. |
| `TestAccSubscriptionGroupLocalizationResource_basic` | Net zero | A group plus one `en-US` group localization; renames it, adds a `custom_app_name`, then imports it both by bare ID and as `<group>/<localization>`. | A group appears with a customer-facing display name, which changes, then both are deleted. |
| `TestAccSubscriptionGroupLocalizationResource_requiresReplace` | Net zero | A group plus a group localization, replaced by one for a different locale (`en-US` → `en-GB`). | One localization appears, is destroyed and replaced by another under a second locale. |
| `TestAccSubscriptionResource_basic` | Net zero | A group plus one subscription; renames it and changes its period from `ONE_MONTH` to `ONE_YEAR`; imports by bare ID. | A subscription appears inside the group showing *Missing Metadata*, its name and duration change, then both are deleted. |
| `TestAccSubscriptionResource_completesMetadata` | Net zero | A group, a subscription, an `en-US` localization, a USA availability, and a USA price read from the price point catalogue. Imports all three children. | A subscription that leaves *Missing Metadata* once its name and price are set. All of it is then deleted. |
| `TestAccSubscriptionPriceScheduleResource_basic` | Net zero | A group, a subscription, a USA+GBR availability, and a price schedule carrying a USA price plus the GBR point Apple equalizes from it — written in a single `PATCH`. Imports by the subscription ID, then adds a future-dated USA increase. | A subscription priced in two storefronts at once, then in three. All of it is then deleted. |
| `TestAccSubscriptionPromotionalOfferResource_basic` | Net zero | A group, a subscription, a USA+GBR availability, and a `PAY_AS_YOU_GO` promotional offer priced in USA plus the GBR point Apple equalizes from it. Drops GBR in place, lists the offer through `apple_subscription_promotional_offers`, then imports it by bare ID and as `<subscription>/<offer_code>`. The offer code is randomised. | A subscription with a promotional offer in two storefronts, then one. All of it is then deleted, the offer first. |
| `TestAccSubscriptionPromotionalOfferResource_freeTrial` | Net zero | A group, a subscription, a USA availability, and a `FREE_TRIAL` promotional offer whose one price names the territory and no price point. Run this first: it is the one shape Apple's schema permits but the provider has not seen accepted. | A subscription with a one-month free promotional offer, then all of it deleted. |
| `TestAccSubscriptionPromotionalOfferResource_validation` | Plan-only | Nothing. A paid offer without a price point, an unknown duration and an offer code with a slash are rejected before Apple is asked. | No change. |
| `TestAccSubscriptionIntroductoryOfferResource_freeTrial` | Net zero | A group, a subscription, a USA+GBR+CAN availability and price schedule, and a one-week free trial in USA and GBR — two records in one `PATCH` — read back through `apple_subscription_introductory_offers`. Imports from the GBR record, then sets an end date and swaps GBR for CAN in place. | A subscription with an introductory offer in two storefronts, then an end date on it and a different second storefront. All of it is then deleted. |
| `TestAccSubscriptionIntroductoryOfferResource_payAsYouGo` | Net zero | The same sellable subscription, plus three discounted months at the USA 0.99 point and its GBR equalization. Imports from the USA record. | A pay-as-you-go introductory offer in two storefronts. All of it is then deleted. |
| `TestAccSubscriptionIntroductoryOfferResource_validation` | Plan-only | Nothing — runs without credentials. | No change. |
| `TestAccSubscriptionResource_requiresReplace` | Net zero | A group plus a subscription, replaced by one with a different product ID. **Consumes two identifiers rather than one.** | One subscription appears, then is destroyed and replaced by another. |
| `TestAccSubscriptionResource_validation` | Plan-only | Nothing. | No change. |
| `TestAccAppsDataSource_basic` | Read-only | Nothing — apps cannot be created by the API. | No change. |
| `TestAccAppsDataSource_filtering` | Read-only | Nothing. | No change. |
| `TestAccSubscriptionGroupsDataSource_basic` | Net zero | One group, read back through two data sources. | A group appears and is deleted. |
| `TestAccSubscriptionsDataSource_basic` | Net zero | A group plus two subscriptions at group levels 1 and 2, read back through five data sources. | Two subscriptions appear in one group, then all are deleted. |
| `TestAccSubscriptionPricePointsDataSource_basic` | Net zero | A group plus one subscription; reads Apple's price catalogue for USA and for USA+GBR. | A subscription appears and is deleted. The catalogue is read-only. |
| `TestAccSubscriptionPricePointEqualizationsDataSource_basic` | Net zero | A group plus one subscription; reads the USA 9.99 price point, then the points Apple equalizes from it — once for every territory and once narrowed to GBR+EGY. | A subscription appears and is deleted. Nothing is priced: the equalization is a read of Apple's price matrix, not a price record. |

Three API shapes explain most failures here. Apple publishes **no top-level
collection** for subscription groups, subscriptions or localizations — `GET
/v1/subscriptions` is a 404 — so everything is reached through its parent. It
publishes **no `GET` for a single subscription price**, so a price is read by
listing its subscription's schedule and scanning, and imports as
`<subscription_id>/<price_id>`. And it reports **no app linkage on a subscription
group**: `include=app` is not among the accepted values, so a group imports as
`<app_id>/<group_id>` and `app_id` is never refreshed from Apple.

A price point ID encodes the subscription it belongs to, so one read from a
different subscription is rejected. That is why
`TestAccSubscriptionResource_completesMetadata` reads the catalogue through a
data source in the same configuration rather than hardcoding an ID.

`TestAccSubscriptionPriceScheduleResource_basic` is the bulk path and checks two
things state alone cannot. `testAccCheckSubscriptionPriceCount` asks Apple how
many prices the subscription actually carries, because a `PATCH` Apple accepted
but committed partially is a failure mode the per-territory resource does not
have. And its import step uses `ImportStateCheck` rather than
`ImportStateVerify`: Apple reports a territory and a plan type on every price
whether or not one was configured, so an imported set is richer than the one
that was written and `ImportStateVerify` would read that as a mismatch. Its
update step **adds** a price rather than removing one — removal runs into Apple's
rule that only future price changes can be deleted, which a test cannot
usefully assert against a price that went live the moment it was written.

**A subscription cannot be priced before it is available.** Apple rejects
`POST /v1/subscriptionPrices` with a 409 reading only "an error occurred while
processing the pricing information" until the subscription has a
`subscriptionAvailabilities` record, and names neither the availability nor the
territory. Nothing in a price references an availability, so
`testAccSubscriptionCompleteConfig` declares `depends_on` to order them; without
it Terraform is free to create the price first and the test fails
intermittently. The availability is one of the two resources in the suite that
cannot be deleted in its own right — destroying the subscription takes it along,
which is why the test is still net zero. The price is the other: once it is in
effect Apple deletes only future price changes, so `terraform destroy` warns and
drops it from state, and the subscription's own deletion is what actually
removes it.

The introductory offer tests check Apple rather than state the same way.
`testAccCheckIntroductoryOfferCount` counts **every** introductory offer record
on the subscription, not just the resource's: the bulk create repeats the
subscription's existing offers in its `PATCH` so a replacing write keeps them,
and a removed territory is a `DELETE`, so either going wrong shows up as a count
that is off. Their import steps ignore `start_date` and
`target_subscription_plan_type`, which import adopts from Apple and Read never
refreshes. Destroying the offer deletes its records; destroying the subscription
would take them along anyway.
### Win-back offers

**These need a subscription App Review has approved**, which the suite cannot
make: Apple creates a win-back offer only on an approved subscription, and the
API has no way to get one approved. `TestAccWinBackOfferResource_basic` therefore
skips unless `APPLE_TEST_APPROVED_SUBSCRIPTION_ID` names one — credentials and a
test app are deliberately not enough. Point it at a subscription whose lapsed
customers you do not mind being eligible for a test offer in a month's time:
the offer starts thirty days out and is deleted at the end of the run, so no
customer should ever see it, but an interrupted run leaves a scheduled offer
that will go live.

**The `offer_id` is treated as reserved forever.** Apple says it "must not have
already been used for another offer in the same subscription group" — *used*,
not *in use* — so each run randomises one of the form `tf_winback_<random>` the
way product identifiers are randomised, and consumes it.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccWinBackOfferResource_basic` | Net zero — opt-in, **consumes an `offer_id`** | One win-back offer on `APPLE_TEST_APPROVED_SUBSCRIPTION_ID`, priced in the USA at the 0.99 price point, starting in 30 days. Changes its priority, end date and wait between offers in place; reads it through `apple_subscription_win_back_offers`; imports it as `<subscription_id>/<offer_id>`; deletes it. | A scheduled win-back offer appears under the subscription's offers, changes, then is deleted. |
| `TestAccWinBackOfferResource_validation` | Plan-only | Nothing. | No change. |

## In-app purchases

**These tests need the same app the subscription tests do**, and skip without
`APPLE_TEST_APP_ID` for the same reason — `testAccPreCheckInAppPurchase` is a
thin wrapper over `testAccPreCheckSubscription`.

These are one-time purchases: consumables, non-consumables and non-renewing
subscriptions. They are a *different* Apple resource from auto-renewable
subscriptions and appear in a different part of App Store Connect, so a leftover
from one never blocks the other.

**Apple never releases an in-app purchase product identifier either**, so these
tests randomise theirs the way the subscription tests do — each run permanently
consumes a few identifiers of the form `com.test.terraform.iap<random>`.

Everything is deletable while it has never been approved. Two of the resources
have no `DELETE` of their own — a price schedule and an availability record —
but deleting the purchase takes them with it, so a completed run is still net
zero. An *interrupted* run is the case to watch: a purchase left behind carries
its price and availability with it, and both are visible in the portal.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccInAppPurchaseResource_basic` | Net zero | One non-consumable; renames it, turns on Family Sharing and adds a review note; imports as `<app>/<purchase>`. | A purchase appears showing *Missing Metadata*, its name changes, then it is deleted. |
| `TestAccInAppPurchaseResource_importRejectsBareID` | Net zero | One non-consumable; then attempts a bare-ID import and expects it to be refused. | A purchase appears and is deleted. The failed import changes nothing. |
| `TestAccInAppPurchaseResource_completesMetadata` | Net zero | A purchase, an `en-US` localization, a USA price read from the price point catalogue, and availability in USA only. Imports all three children. | A purchase with a name, a price and one territory, all of it then deleted. |
| `TestAccInAppPurchaseResource_validation` | Plan-only | Nothing. | No change. |
| `TestAccInAppPurchasesDataSource_basic` | Net zero | Two purchases — one non-consumable, one consumable — read back through three data sources. | Two purchases appear and are deleted. |
| `TestAccInAppPurchasePricePointsDataSource_basic` | Net zero | One purchase; reads Apple's price catalogue for USA and for USA+GBR. | A purchase appears and is deleted. The catalogue is read-only. |

Three API shapes explain most failures here, and they are not the subscription
ones. The purchase record has **no app relationship at all**, so nothing can be
imported without the app ID and `app_id` is never refreshed. Localizations hang
off an **in-app purchase version** rather than the purchase, so the provider
resolves one and may create it — versions cannot be deleted, and one created this
way survives the test that caused it. And the price schedule and availability are
**singular records replaced by a `POST`** with no `PATCH` and no `DELETE`, so a
`terraform destroy` of either alone only drops state and warns.

A price point ID encodes the purchase it belongs to — and a subscription's price
points are meaningless here — which is why
`TestAccInAppPurchaseResource_completesMetadata` reads the catalogue through a
data source in the same configuration rather than hardcoding an ID.

## App listing metadata

**These tests need the same app the subscription tests do**, and skip without
`APPLE_TEST_APP_ID` — `testAccPreCheckSubscription` is the check.

They are unlike everything else in this suite, and the difference matters before
you run them. Every other test *creates* something and deletes it. Most of these
**edit the app itself**, and cannot put it back.

**Point `APPLE_TEST_APP_ID` at a scratch app.** These tests rewrite its
categories, its age rating answers, and its localized name and subtitle. Apple
creates the app info, the age rating declaration and the app record with the app
and publishes no `POST` and no `DELETE` for them, so the provider adopts them and
`terraform destroy` only drops state — the values the test wrote stay on the app
afterwards. `testAccCheckAppStillExists` is the only honest `CheckDestroy` for
those: there is nothing to assert gone, so it asserts the app survived.

**Two tests are guarded past credentials.** `TestAccAppPriceScheduleResource_basic`
and `TestAccAppAvailabilityResource_basic` change what the app costs and which
storefronts sell it, and Apple publishes no `DELETE` for either record. They skip
unless `APPLE_TEST_ALLOW_APP_PRICING` is set. Never point them at a live app: on
one, they would change the price customers pay and remove storefronts from sale.

**`TestAccAppStoreVersionResource_build` needs a build that already exists.** The
provider does not upload builds — only Xcode, Transporter and fastlane do — so
unlike the rest of the suite this test cannot manufacture its own fixture. It
skips unless `APPLE_TEST_BUILD_NUMBER` and
`APPLE_TEST_BUILD_PRE_RELEASE_VERSION` name a build already uploaded to the app
under test and finished processing; Apple rejects a build that is still
`PROCESSING`, which lasts five to thirty minutes after the upload. Its version
string is **not randomised**, because Apple only accepts a build whose train
matches the version string — so the version is named after the build, and an
interrupted run leaves one to delete in App Store Connect before it passes
again. It attaches and then detaches; the build itself is never modified.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccAppSettingsResource_contentRights` | **Leaves the declaration set** | Nothing. Patches the app record's content rights declaration, then flips it to `USES_THIRD_PARTY_CONTENT`, then imports. | App Information > Content Rights changes twice and stays at the second value. |
| `TestAccAppInfoResource_categories` | **Leaves the categories set** | Nothing. Sets Finance + Productivity, then Productivity alone, then imports. | App Information > Category changes twice; the secondary category ends up cleared. |
| `TestAccAppInfoLocalizationResource_basic` | **Leaves the name and subtitle set** | An `en-US` app info localization if one did not exist. Sets the name to `TF Acceptance App` once and varies the subtitle. | The app's displayed name and subtitle change. The name is fixed, so a second run does not rename it again. |
| `TestAccAgeRatingDeclarationResource_basic` | **Leaves the answers set** | Nothing. Answers the questionnaire, then raises cartoon violence to `INFREQUENT_OR_MILD`, then imports. | App Information > Age Rating changes, and Apple recomputes the rating from it. |
| `TestAccAppStoreVersionResource_basic` | Net zero | One iOS version at `99.x.y`; updates its copyright and release type; imports. | A version appears under the app, then is deleted. |
| `TestAccAppStoreVersionResource_build` | Net zero — opt-in | One iOS version named after the build's train; attaches the build named by `APPLE_TEST_BUILD_NUMBER`, then detaches it by removing the attribute. | A version appears with a build attached, loses it, then is deleted. The build itself is untouched. |
| `TestAccAppStoreVersionLocalizationResource_basic` | Net zero | A version plus its `en-US` product page; changes the keyword list from three to two. | A version with a description and keywords appears, then is deleted. |
| `TestAccAppStoreVersionLocalizationResource_keywordsTooLong` | Plan-only | Nothing. Twelve keywords that join past 100 characters are refused at plan time. | No change. |
| `TestAccAppStoreReviewDetailResource_basic` | Net zero | A version plus its review contact and notes; updates the notes; imports. | App Review Information appears under the version, then goes with it. |
| `TestAccAppPriceScheduleResource_basic` | **Leaves the app priced** — opt-in | A price schedule at the USA zero price point. | Pricing and Availability shows the app as free. There is no way to undo this. |
| `TestAccAppAvailabilityResource_basic` | **Leaves availability set** — opt-in | Availability in USA + GBR, then USA + GBR + DEU. | Pricing and Availability lists three storefronts. There is no way to undo this. |
| `TestAccAppCategoriesDataSource_basic` | Net zero | Nothing — and needs no app, only credentials. Reads Apple's category catalogue. | No change. |
| `TestAccAppPricePointsDataSource_basic` | Net zero | Nothing. Reads the app's price catalogue for USA. | No change. |
| `TestAccAppStoreVersionsDataSource_basic` | Net zero | Nothing. Lists the app's iOS versions. | No change. |

The version tests are the only ones here that create a deletable record, and
only while it is still being prepared: a version that reached review cannot be
removed, and `Delete` warns rather than erroring. Version strings are randomised
in the `99.x.y` range because an app holds **one editable version per platform at
a time** — a fixed one would collide with whatever an interrupted run left
behind, and Apple answers that with a 409.

An interrupted run leaves at most one version, which takes its localization and
review detail with it when deleted. Everything else was an edit, not a creation.

**A note on timing.** Apple freezes the app info once a version is in review or
distributed, and publishes no way to create a fresh one. If the scratch app has a
version in review, the four editing tests above fail with *No Editable App Info*
rather than doing anything harmful. Wait for review to finish.

## TestFlight

**These tests need the same app the subscription and app listing tests do**, and
skip without `APPLE_TEST_APP_ID` — `testAccPreCheckSubscription` is the check.

They sit between the two halves of this suite. Beta groups are created and
deleted like a subscription, so a completed run is neutral; the beta review
detail is adopted like an app info, so what it writes stays on the app.

**No test opens a public TestFlight link.** Enabling `public_link_enabled`
issues a URL anyone can join the beta through, which is not something a test
should put into the world. The one configuration that would be rejected for it —
a public link on an internal group — is checked at plan time and never applied.

**Group names are randomised** (`Terraform External <6 chars>`), because Apple
enforces the name unique within an app: a fixed name would collide with whatever
an interrupted run left behind. A group with no testers and no public link
invites nobody, so an interrupted run leaves something to tidy rather than
something to worry about.

**The TestFlight text is written in `de-DE`, not `en-US`.** Apple writes a
localization for the app's primary locale itself and refuses to delete the last
one an app has, so a test on the primary locale would adopt a record it could
not then remove.

**`TestAccBetaBuildLocalizationResource_basic` needs a build that already
exists**, for the same reason `TestAccAppStoreVersionResource_build` does: the
provider does not upload builds. It skips unless `APPLE_TEST_BUILD_NUMBER` and
`APPLE_TEST_BUILD_PRE_RELEASE_VERSION` name a build already uploaded to the app
under test.

**`TestAccBetaTesterResource_basic` emails a real person**, which is why it is
guarded past credentials like the app pricing tests. Creating a membership makes
Apple send a TestFlight invitation, so there is no safe default address: the test
skips unless `APPLE_TEST_BETA_TESTER_EMAIL` names one the runner is entitled to
invite — your own address is the obvious choice. Destroy removes the tester from
the group, not from the account: Apple's `DELETE /v1/betaTesters/{id}` would
remove the person from every app in the team, so the provider never calls it and
the record stays under TestFlight > Testers afterwards.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccBetaGroupResource_basic` | Net zero | One external beta group with a randomised name and no testers; renames it and turns feedback off; imports by bare ID. | A group appears under TestFlight > Groups, is renamed, then is deleted. |
| `TestAccBetaGroupResource_internal` | Net zero | One internal beta group with `has_access_to_all_builds`; imports by bare ID. | An internal group appears under TestFlight > Testers, then is deleted. |
| `TestAccBetaGroupResource_publicLinkOnInternal` | Plan-only | Nothing. A public link on an internal group is refused at plan time. | No change. |
| `TestAccBetaTesterResource_basic` | **Leaves the tester record** — opt-in | One external group with a randomised name, plus a membership for `APPLE_TEST_BETA_TESTER_EMAIL`; imports by `<group_id>/<email>`. Apple emails that address an invitation. | A tester appears in the group under TestFlight > Testers and Groups, then the group and the membership go — the tester stays in the account's tester list. |
| `TestAccBetaAppLocalizationResource_basic` | Net zero | A `de-DE` TestFlight page description, feedback email and marketing URL; updates the description; imports. | TestFlight > Test Information gains a German entry, then loses it. |
| `TestAccBetaBuildLocalizationResource_basic` | Net zero — opt-in | A `de-DE` "What to Test" note on the build named by `APPLE_TEST_BUILD_NUMBER`; updates it; imports by the composite ID. | The build's What to Test gains a German entry, then loses it. The build itself is untouched. |
| `TestAccBetaAppReviewDetailResource_basic` | **Leaves the beta review contact set** | Nothing. Apple publishes no `POST` and no `DELETE` for the record, so this patches what is already there and `destroy` only drops state. | TestFlight > Test Information > Beta App Review Information shows the test's contact details and notes, permanently. |

An interrupted run leaves at most a beta group or two, deletable in TestFlight >
Groups, and possibly a `de-DE` entry under Test Information. Neither blocks a
re-run: group names are randomised, and the localizations are adopted rather
than created.

## Team members (Users and Access)

**These are the only tests in the suite that can take somebody's access away**,
and both write to the team rather than to an app, so `APPLE_TEST_APP_ID` is
irrelevant to them. They need an App Store Connect API key with the **Admin**
role: an App Manager key can read `/v1/users` and not write to it, so every
write here fails with a 403 on a key that runs the rest of the suite.

**`TestAccUserInvitationResource_basic` emails a real person**, the way the
TestFlight tester test does, and is guarded the same way: it skips unless
`APPLE_TEST_USER_INVITE_EMAIL` names an address the runner is entitled to
invite. Destroy cancels the invitation, so a completed run is neutral; an
interrupted one leaves a pending invitation under Users and Access, which lapses
by itself after 72 hours. **Do not let the address accept it mid-run** — an
accepted invitation cannot be cancelled, the destroy warns instead of erroring,
and whoever accepted is then a member of the team with the role the test asked
for.

**`TestAccUserResource_basic` removes a member from the team on destroy**, and
Apple publishes no way to undo that: the person has to be invited again and
accept again. It is guarded past credentials twice over — `APPLE_TEST_USER_EMAIL`
names the member and `APPLE_TEST_ALLOW_USER_REMOVAL` says removing them is
acceptable — because naming somebody is not on its own a statement that losing
them is. Point it at an address you control that is on the team for this purpose
and nothing else. **Never point it at a colleague.** An interrupted run leaves
that member holding `DEVELOPER` and `APP_MANAGER` with provisioning access,
which is the last thing the test applied.

The roles the tests grant stop at `APP_MANAGER` on purpose. A test that granted
`ADMIN` would open a window in which the member could do anything to the team,
and an interrupted run would leave it open.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccUsersDataSource_basic` | No writes | Nothing. Lists the team and asserts it is not empty. | No change. |
| `TestAccUsersDataSource_filtered` | No writes | Nothing. Reads the team three times with different filters. | No change. |
| `TestAccUserResource_notOnTheTeam` | No writes | Nothing. An address that is not a member is refused before any write, because Apple cannot create a user. | No change. |
| `TestAccUserInvitationResource_accountHolderRole` | Plan-only | Nothing. `ACCOUNT_HOLDER` is refused at plan time. | No change. |
| `TestAccUserInvitationResource_conflictingVisibility` | Plan-only | Nothing. `all_apps_visible` with `visible_apps` is refused at plan time. | No change. |
| `TestAccUserInvitationResource_basic` | Net zero — opt-in, **emails a person** | One pending invitation for `APPLE_TEST_USER_INVITE_EMAIL` with the `DEVELOPER` role; imports by the address; cancels it. | Users and Access > Users shows a pending invitation, then loses it. The address receives an invitation email that then stops working. |
| `TestAccUserResource_basic` | **Removes the member** — doubly opt-in | Nothing new. Changes `APPLE_TEST_USER_EMAIL`'s roles to `DEVELOPER`, then to `DEVELOPER` + `APP_MANAGER` with provisioning allowed; imports by the address; then **deletes them from the team**. | That member's roles change under Users and Access, then the member disappears. |

An interrupted run leaves at most a pending invitation to cancel, or a member
carrying the roles the last step applied. Neither blocks a re-run.

## Merchant IDs

Deletable. Apple rejects duplicates, so leftovers block re-runs.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccMerchantIDResource_basic` | Net zero | `merchant.com.test.terraform`, renamed "Terraform Test Merchant" → "Renamed Test Merchant". | A Merchant ID appears, its display name changes, then it is deleted. |
| `TestAccMerchantIDResource_importByAppleID` | Net zero | `merchant.com.test.terraform-import`. | One Merchant ID appears and is deleted. |
| `TestAccMerchantIDResource_requiresReplace` | Net zero | `merchant.com.test.terraform-before`, replaced by `…-after`. | Two Merchant IDs appear in sequence, both deleted. |
| `TestAccMerchantIDResource_validation` | Plan-only | Nothing. | No change. |
| `TestAccMerchantIDsDataSource_basic` | Net zero | `merchant.com.test.terraform-ds`. | One Merchant ID appears and is deleted. |
| `TestAccMerchantIDsDataSource_filtering` | Net zero | `…terraform-ds-alpha` and `…terraform-ds-beta`. | Two Merchant IDs appear and are deleted. |
| `TestAccMerchantIDsDataSource_validation` | Plan-only | Nothing. | No change. |

## Pass Type IDs

`testAccCheckPassTypeIDDestroy` only inspects Terraform state, so it cannot
catch a Pass Type ID that survived at Apple — verify in the portal after a run.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccPassTypeIDResource_basic` | Net zero | `pass.com.test.terraform-example`, renamed "Test Example Pass" → "Updated Example Pass". | A Pass Type ID appears, its name changes, then it is deleted. |
| `TestAccPassTypeIDResource_importByIdentifier` | Net zero | `pass.com.test.terraform-import`. | One Pass Type ID appears and is deleted. |
| `TestAccPassTypeIDResource_invalidIdentifier` | Plan-only | Nothing — identifier must start with `pass.`. | No change. |
| `TestAccPassTypeIDResource_invalidName` | Plan-only | Nothing. | No change. |
| `TestAccPassTypeIDsDataSource_basic` | Read-only | Nothing. Lists every Pass Type ID. | No change. |
| `TestAccPassTypeIDsDataSource_withFilters` | Read-only | Nothing. | No change. |
| `TestAccPassTypeIDsDataSource_withSorting` | Read-only | Nothing. | No change. |
| `TestAccPassTypeIDsDataSource_withLimit` | Read-only | Nothing. | No change. |

## Provisioning profiles

The heaviest tests: each builds a Bundle ID, an `IOS_DISTRIBUTION` certificate
and a profile. `IOS_APP_STORE` is deliberate — development and ad hoc profiles
require devices, and devices are permanent.

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccProfileResource_basic` | Slot, returned | `com.test.terraform-profile` + 1 distribution certificate + profile "Terraform Test Profile"; imported by ID and by name. | An App ID, a Distribution certificate and an *App Store* profile all appear, then all three are removed. |
| `TestAccProfileResource_rename` | Slot, returned | Same trio; profile renamed "Terraform Rename Before" → "Terraform Rename After", which reissues it. | The first profile disappears and a second appears under the new name. Apple rejects `PATCH /v1/profiles` with 403 `does not allow 'UPDATE'`, so `name` is `RequiresReplace`. |
| `TestAccProfileResource_requiresReplace` | Slot, returned | Two Bundle IDs (`…-first`, `…-second`) + 1 certificate; the profile is moved between them, forcing a recreate. | Two App IDs, one certificate, and a profile that is deleted and reissued against the second App ID. |
| `TestAccProfileResource_validation` | Plan-only | Nothing. | No change. |
| `TestAccProfilesDataSource_basic` | Slot, returned | `com.test.terraform-profile-ds` + 1 certificate + profile "Terraform DS Profile"; read back through five data sources. | An App ID, a Distribution certificate and a profile appear, then all are removed. |
| `TestAccProfilesDataSource_validation` | Plan-only | Nothing. | No change. |

## Provider

| Test | Residue | Creates at Apple | What you see in the portal |
|---|---|---|---|
| `TestAccProvider` | No calls | Applies an empty config. The JWT is minted locally; no request is sent. | No change. |
| `TestAccProvider_ConfigurationValidation` | No calls | Nothing — three deliberately malformed credential sets, all rejected by validators before any request. Runs even without credentials. | No change. |

## If a run is interrupted

Identifiers are fixed rather than randomised, so leftovers block the next run.
Delete these in the portal before retrying:

```
Identifiers → App IDs
  com.test.terraform-example         com.test.original
  com.test.changed                   com.test.disappear
  com.test.datasource                com.test.filter.ios
  com.test.filter.macos              com.test.sort.aaa
  com.test.sort.zzz                  com.test.platform-ios
  com.test.platform-mac-os           com.test.platform-tv-os
  com.test.platform-watch-os         com.test.terraform-capability
  com.test.terraform-capability-ds   com.test.terraform-profile
  com.test.terraform-profile-first   com.test.terraform-profile-second
  com.test.terraform-profile-ds

Identifiers → Merchant IDs
  merchant.com.test.terraform          merchant.com.test.terraform-import
  merchant.com.test.terraform-before   merchant.com.test.terraform-after
  merchant.com.test.terraform-ds       merchant.com.test.terraform-ds-alpha
  merchant.com.test.terraform-ds-beta

Identifiers → Pass Type IDs
  pass.com.test.terraform-example      pass.com.test.terraform-import

Profiles
  Terraform Test Profile               Terraform Rename Before / After
  Terraform Replace Profile            Terraform DS Profile

Certificates
  Any certificate whose CSR common name is
  "terraform-provider-apple acceptance test"  → revoke it

App Store Connect → the APPLE_TEST_APP_ID app → Monetization → Subscriptions
  Any group named "Terraform Test / Renamed / Import / Sub / Full /
  Replace / DS / SubDS / PP <random>"     → delete it, and the
                                             subscriptions inside it first

App Store Connect → APPLE_TEST_APPROVED_SUBSCRIPTION_ID → Win-Back Offers
  Any offer whose offer ID starts with
  "tf_winback_"                           → delete it before its start date,
                                             or it goes live to lapsed
                                             subscribers.

App Store Connect → the APPLE_TEST_APP_ID app → Monetization → In-App Purchases
  Any purchase whose product ID starts with
  "com.test.terraform.iap"                → delete it. Its price and
                                             availability go with it; neither
                                             can be deleted on its own.

App Store Connect → the APPLE_TEST_APP_ID app → the version list
  Any iOS version numbered "99.x.y"       → delete it, if it is still in
                                             Prepare for Submission. Its
                                             localization and review detail go
                                             with it.

App Store Connect → the APPLE_TEST_APP_ID app → TestFlight → Testers and Groups
  Any group named "Terraform External /   → delete it. It has no testers and
  Internal / Renamed / Invalid <random>"     no public link, so deleting it
                                             invites and uninvites nobody.

App Store Connect → the APPLE_TEST_APP_ID app → TestFlight → Test Information
  A German (de-DE) entry beginning         → delete it if you want the app's
  "Terraform acceptance test build"          Test Information tidy. It is
                                             harmless left in place.

App Store Connect → Users and Access → TestFlight → Testers
  The address named by                     → remove it if you want the tester
  APPLE_TEST_BETA_TESTER_EMAIL, if that      list tidy. The provider never
  test ran                                   deletes a tester record, because
                                             Apple's delete removes the person
                                             from every app in the team.
```

The app listing tests leave more than that behind, and none of it is cleanup in
the usual sense — it is the app's own metadata, edited in place. After a run the
scratch app's content rights declaration, categories, age rating answers,
localized name and subtitle are whatever the tests last set. There is nothing to
delete; set them back by hand if you care what the scratch app says. The beta
review contact and notes under TestFlight > Test Information are in the same
position: Apple publishes no `DELETE` for that record either.

App Store Connect leftovers do not block the next run the way the others do:
group reference names and every product identifier are randomised, so a second
run collides with nothing. Clean them up anyway — an abandoned subscription or
purchase still shows in the portal, and neither can be deleted once it has been
approved. An interrupted in-app purchase run may also leave an extra *in-app
purchase version* behind; versions have no delete endpoint and are harmless.

## How to run it

These tests never run automatically. In CI they are a manual
`workflow_dispatch` of **Acceptance Tests** (`.github/workflows/acceptance.yml`),
which takes a Terraform version, a `-run` pattern, and an `include_device_tests`
toggle. Locally, export the three credentials the provider reads, then start
with the tests that leave nothing behind before committing the device slots.

```bash
export APPLE_APP_STORE_CONNECT_ISSUER_ID=...
export APPLE_APP_STORE_CONNECT_API_KEY=...
export APPLE_APP_STORE_CONNECT_PRIVATE_KEY="$(cat AuthKey_XXXXXXXXXX.p8)"

# Optional. Without it every TestAccSubscription*, TestAccInAppPurchase*, and
# TestAccApps* and TestAccBeta* test skips itself, because Apple's API cannot create the app
# record they hang off.
export APPLE_TEST_APP_ID=6448459855

# Optional. Without it TestAccWinBackOfferResource_basic skips itself: Apple
# creates win-back offers only on a subscription App Review has approved.
export APPLE_TEST_APPROVED_SUBSCRIPTION_ID=6447497832

# 1. Read-only first — proves the credentials work, creates nothing.
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccProvider|DataSource_validation|DataSource_empty|DevicesDataSource'

# 2. Everything except the two permanent device tests.
#
#    The app listing tests run here, and they edit the APPLE_TEST_APP_ID app's
#    own metadata in place -- categories, age rating, localized name -- with no
#    way to put it back. Use a scratch app.
TF_ACC=1 go test -v ./internal/provider/ -timeout 120m \
  -run 'TestAcc' -skip 'TestAccDeviceResource'

# 3. Only when you have accepted the permanent slots.
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m -run 'TestAccDeviceResource'

# 4. Only on an app whose price and storefronts you are willing to change
#    permanently. Apple publishes no DELETE for either record.
export APPLE_TEST_ALLOW_APP_PRICING=1
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccAppPriceScheduleResource|TestAccAppAvailabilityResource'

# 5. Only when a build has already been uploaded to the test app and has
#    finished processing. Name the build the way the pipeline that built it
#    does: CURRENT_PROJECT_VERSION and MARKETING_VERSION.
export APPLE_TEST_BUILD_NUMBER=42
export APPLE_TEST_BUILD_PRE_RELEASE_VERSION=1.2.0
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccAppStoreVersionResource_build'

# 6. Only with an address you are entitled to invite — applying it sends a
#    real TestFlight invitation, and the tester record stays in the account
#    afterwards.
export APPLE_TEST_BETA_TESTER_EMAIL=you@example.com
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccBetaTesterResource'

# 7. Only with an address you are entitled to invite — applying it emails that
#    address an invitation to join the team. Needs an Admin API key.
export APPLE_TEST_USER_INVITE_EMAIL=you@example.com
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccUserInvitationResource'

# 8. Only with a team member you are willing to lose. Destroy removes them from
#    the team and revokes their access to every app, and the API cannot undo it.
#    Never point this at a colleague.
export APPLE_TEST_USER_EMAIL=scratch-member@example.com
export APPLE_TEST_ALLOW_USER_REMOVAL=1
TF_ACC=1 go test -v ./internal/provider/ -timeout 30m \
  -run 'TestAccUserResource_basic'
```

Steps 6 and 7 are the two tests that send mail to a person, which is why
credentials and a test app are not enough to run either. Without
`APPLE_TEST_BETA_TESTER_EMAIL` and `APPLE_TEST_USER_INVITE_EMAIL` they skip, so
step 2 stays safe — as does step 8, which needs two variables rather than one
because it ends by removing somebody from the team.

Step 5 is skipped rather than failed when those two variables are absent,
because nothing in the suite can produce their fixture: Apple publishes no
build-upload endpoint, so the binary has to arrive through Xcode, Transporter or
fastlane first.

Step 4 is opt-in past credentials for a reason: those two tests are the only
ones in the suite that change what an app costs and where it sells, and neither
record can be deleted. Without `APPLE_TEST_ALLOW_APP_PRICING` they skip, which
is why step 2 is safe to run on a scratch app that is otherwise configured.

Step 3 passes once per team and never again: the UDIDs are hardcoded and destroy
only disables the device, so a second run gets Apple's 409 back as
`Device Already Exists`. The CI equivalent is step 2 —
`.github/workflows/acceptance.yml` passes `-skip 'TestAccDeviceResource'` unless
its `include_device_tests` input is set.

`make testacc` runs everything, devices included. Prefer the staged commands
above for a first run against a new team.

Do not add `-parallel`: the suite uses fixed identifiers that would collide.
For the same reason CI runs one Terraform version per dispatch rather than a
matrix, and its `concurrency` group queues a second run instead of starting it
alongside the first.

If Go's test timeout kills a run mid-apply, Terraform never destroys — that is
the main way you end up with the leftovers listed above.

## Keeping this file honest

It is hand-written and nothing verifies it. When you add or rename an
acceptance test, update the matching table. The counts in
[What a clean full run costs](#what-a-clean-full-run-costs) come from the
per-section tables, so recompute them if a residue classification changes.
