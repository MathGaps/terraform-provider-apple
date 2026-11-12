# apple_subscription_price_schedule: change the price of an approved subscription (0.6.0)

## Why

iac#487 (PRO monthly 8 -> 9 USD) failed on prod with HTTP 409 "Initial price cannot be created again after
subscription is approved". `write()` always sends the replace-all `PATCH /v1/subscriptions/{id}` with undated
prices (`SetSubscriptionPrices`, internal/apple/subscriptionPrices.go). Apple reads that as an *initial* price,
which it accepts only in MISSING_METADATA / READY_TO_SUBMIT / DEVELOPER_ACTION_NEEDED. For every other state a
price change is a new record: `POST /v1/subscriptionPrices`, one per territory.

## What changes

1. `write()` (internal/provider/subscription/price_schedule_resource.go) first reads the subscription
   (`GetSubscription`, `attributes.state`).
   - MISSING_METADATA, READY_TO_SUBMIT, DEVELOPER_ACTION_NEEDED: unchanged PATCH.
   - Any other state: `GetSubscriptionPrices`, reduce to the effective price per territory (reconcile rule
     below), and `CreateSubscriptionPrice` once for each planned territory whose price point differs from the
     effective one. Attributes: `startDate` = `start_date` (omitted when unset, which the existing
     `CreateSubscriptionPrice` already does), `preserveCurrentPrice` = `preserve_current_price`, `planType`
     as configured. A territory in state but absent from the plan is not touched (a live price cannot be
     withdrawn; today's PATCH has the same limit). A pure function `pricesToPost(plan, effective)` holds the
     decision so it is testable without HTTP.
   - Territory is `territory_id`, else the one encoded in the price point (price point -> territory via the
     reported price list; if neither is known the error names the price point).
2. `reconcileSchedulePrices` (same file) reduces what Apple reports before matching state.
3. Error text: the default branch no longer says only "in review". It names the subscription's state read in
   step 1, says a price change on a state outside the three above is made by POST and what failed, and for a
   409 "Initial price..." says the provider should have chosen POST (a bug to report). The "in review"
   hint stays for WAITING_FOR_REVIEW / IN_REVIEW / PENDING_BINARY_APPROVAL, where Apple refuses price writes.

Files: `internal/provider/subscription/price_schedule_resource.go` (write, reconcileSchedulePrices, pricesToPost,
descriptions), `internal/apple/subscriptionPrices.go` (no new client call; a `StartDate` is already carried),
`CHANGELOG.md`, generated `docs/resources/subscription_price_schedule.md` (`make generate`).

## Schema

No attribute is added: `prices[].start_date` (Optional, YYYY-MM-DD) and `prices[].preserve_current_price`
(Optional bool) already exist and are in-place (no RequiresReplace; only `subscription_id` forces replacement).
Defaults stay unset. Descriptions change: `start_date` unset = soonest Apple allows (the PATCH path ignores
dates for an initial price); `preserve_current_price` applies on the POST path.

## Reconcile rule

From the prices Apple lists, per territory:
1. Drop records with `preserved == true`.
2. Of the rest keep the one with the greatest `startDate` (null = in force since before any dated record).
   That is the effective price: current, or the pending change if one is scheduled.
3. Match state against these only. A state price whose price point is not effective in its territory is
   dropped from state (it shows as a diff only if the config still wants it).
4. An effective price not in state is adopted with Apple's values, except a *pending* one (`startDate` > today,
   clock injected for tests): adopted with `start_date` null and `preserve_current_price` = the `preserved`
   flag of the record it supersedes, so a change scheduled by hand with the same intent as the configuration
   reads back equal to it.

Required outcome: prod state holds 8, Oli's hand-scheduled 9 exists, config is 9 with `preserve_current_price =
true` and no `start_date` -> Read yields 9/null/true -> empty plan, no POST, no removal of the preserved 8.
Step 4's preserve mapping and step 1's "preserved records are separate rows" depend on question (b) below.

## Tests (3, per CLAUDE.md style: httptest, no credentials)

- `TestWriteInMetadataStatePatches`: subscription READY_TO_SUBMIT -> exactly one PATCH /v1/subscriptions/{id},
  no POST.
- `TestWriteApprovedSubscriptionPostsChangedTerritories`: APPROVED, two territories, one changed -> exactly one
  POST /v1/subscriptionPrices carrying that price point, `preserveCurrentPrice` and, when set, `startDate`;
  no PATCH.
- `TestReconcileIgnoresPreservedAndPendingPrices`: Apple lists old 8 (preserved), current-null and pending 9
  -> state/plan equal to a config of 9, no extra elements.
Tests stay in internal/provider/subscription; they build `&apple.Client{HostURL: srv.URL, HTTPClient: srv.Client(),
Token: "t"}` as internal/apple's `newTestClient` does.

## Release (README/FORK.md/.github/workflows/release.yml)

1. Add a `## 0.6.0 (<date>)` section to CHANGELOG.md (the release workflow fails without a section for the tag
   and uses it as the release notes), `make generate` for docs, `make lint test`.
2. PR to main, squash merge (the 0.5.0 precedent is `chore(release): 0.5.0`).
3. Tag `v0.6.0` on main and push it: the `release` environment (v* tags only) signs, goreleaser builds a draft,
   the workflow publishes once the assets match SHA256SUMS; the OpenTofu registry picks it up within minutes.
   Releases are ingested once and cannot be redone, so the notes and the tag are checked first.

## iac (MathGaps/iac, branch apple-price-change)

- `stacks/appstore/versions.tf` and `stacks/appstore/modules/catalogue/versions.tf`: `~> 0.5.0` -> `~> 0.6.0`
  (+ the comment's "0.5 adds the offers" line); `.terraform.lock.hcl` of the stack updated by `tofu init -upgrade`.
- `stacks/appstore/modules/catalogue/main.tf`: no `start_date` needed (unset = soonest Apple allows);
  `preserve_current_price = true` stays.
- Read-only prod plan of the appstore stack: empty once the hand change is in, otherwise only pro_monthly's
  price scheduling (one POST). The plan needs the iac read key; not run by this role.

## Apple's docs (questions from the lead)

(a) POST with an omitted/null startDate on an approved subscription. Apple's page
`developer.apple.com/documentation/appstoreconnectapi/post-v1-subscriptionprices` gives no state rule;
`SubscriptionPriceCreateRequest.Data.Attributes` (startDate: date, preserveCurrentPrice: boolean, planType) has
every attribute optional. The guide "Configuring subscription prices across territories"
(`.../configuring-subscription-prices-across-territories`) says: "Set `startDate` to `null` to apply the price
immediately, or to a future date to schedule the change." and "Once a price change goes into effect, you can't
revert it." So omitted/null is documented as valid and means immediately; nothing documents a state
restriction. Not proven for APPROVED by a live call.

(b) `GET /v1/subscriptions/{id}/prices` (`.../get-v1-subscriptions-_id_-prices`): documented only as "a list of
prices for an auto-renewable subscription, by territory"; filters are `filter[territory]`,
`filter[subscriptionPricePoint]`, `filter[planType]` (no date/state filter); `SubscriptionPrice.Attributes` are
`startDate` (date), `preserved` (boolean), `planType`; the territory and price point come from the
relationships. The docs do not say whether ended prices are listed or how a preserved one is told apart. Only a
live read can answer; see the note on the item for the call.

## 0.6.2: the change is dated (live finding, 2026-11-12)

Assumption (a) above was wrong. iac#488's prod apply sent 157 `POST /v1/subscriptionPrices` without `startDate` on
an APPROVED subscription and Apple answered 409 "Initial price cannot be created again after subscription is
approved" on the first: an undated price is an initial price. Nothing was written.

What was read (a search, not a live write; Apple publishes no dry run):

- App Store Connect Help, "Manage pricing for auto-renewable subscriptions"
  (developer.apple.com/help/app-store-connect/manage-subscriptions/manage-pricing-for-auto-renewable-subscriptions):
  a *starting price* has no start date, a *price change* on an existing subscription has one chosen; the soonest
  date is decided by time zone, "generally 1 to 2 days in advance"; one future change per storefront and billing
  plan type, and a second overwrites the first; a change in effect cannot be reverted.
- Apple forum 775066 (a POST for a change in its last hour before taking effect: 409 STATE_ERROR "Must delete
  future price change before creating a new price change"): confirms a change is a dated record, one pending per
  territory.
- Apple forum 773452 (PATCH with `prices` refused while in review, "User is not allowed to edit"): no POST detail.
- The "Configuring subscription prices across territories" guide (null startDate = immediately) is contradicted for
  an approved subscription by the 409 above.
- No fastlane/spaceship or asc CLI source stating a minimum date was found; the earliest date is not documented to
  the day.

Decision: an unset `start_date` on the POST path sends UTC today + 3 days (`defaultLeadDays`). The storefront
furthest ahead of UTC (+14h) is a calendar day ahead of the UTC date, so +3 is two local days there and more
elsewhere, which clears "1 to 2 days" everywhere. An explicit `start_date` is sent as written. If Apple still
refuses, the error names the state and the date sent; the lead can be raised in one constant.
Read-back is unchanged: the pending dated record is adopted with `start_date` null and `preserve_current_price`
from the record it supersedes, so the plan after the apply stays empty.
