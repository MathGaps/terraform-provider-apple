# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Terraform provider (`terraform-provider-apple`, module `github.com/AhmedOsman00/terraform-provider-apple`) for Apple App Store Connect, built on the **Terraform Plugin Framework** (not SDK v2). It is published on the Terraform Registry as `ahmedosman00/apple`. It manages Bundle IDs, Bundle ID capabilities, certificates, devices, merchant IDs, Pass Type IDs, and provisioning profiles, plus auto-renewable subscriptions (groups, group localizations, subscriptions, localizations, prices, availability, promotional offers, introductory offers, win-back offers) and one-time in-app purchases (purchases, localizations, price schedules, availability), plus a read-only `apple_territories` listing of the App Store's storefronts. It also manages an app's own App Store listing: content rights, categories, age rating, the localized name and product page, the release record, App Review information, the price and the storefronts it sells in. TestFlight is covered too: tester groups, who is in them, the app's TestFlight page text, a build's "What to Test" note, and what the beta reviewers are told. The team itself is the last layer: who is under Users and Access, what they may do, and the invitations that put them there.

## Commands

```bash
make            # default: fmt + lint + install + generate
make build      # go build -v ./...
make install    # go install -v ./...
make fmt        # gofmt -s -w -e .
make lint       # golangci-lint run  (config in .golangci.yml)
make test       # go test -v -cover -race -timeout=120s -parallel=10 ./...
make testacc    # TF_ACC=1 go test -v -cover -timeout 120m ./...
make generate   # regenerates docs/ from schemas + examples/ (see caveats below)
make validate-examples  # terraform validate over every directory under examples/
```

Run a single test: `go test -v ./internal/apple/ -run TestGetAllPages`, or `TF_ACC=1 go test -v ./internal/provider/ -run TestAccBundleIDResource` for an acceptance test.

`go test ./...` is safe without credentials: acceptance tests skip themselves when the App Store Connect environment variables are absent. Setting `TF_ACC=1` without credentials also skips rather than fails. Acceptance tests hit the real API and create billable resources.

Note that `terraform-plugin-testing` drives a real `terraform` binary even for `resource.UnitTest`, so the provider schema tests need one — it is downloaded automatically if not on PATH.

### `make generate` caveats

`tools/tools.go` drives generation via `go:generate` and requires the `terraform` binary on PATH (`terraform fmt -recursive ../examples/`).

The copywrite step stamps `// Copyright (c) AO Studio` + `// SPDX-License-Identifier: MPL-2.0` headers into source files, configured by `.copywrite.hcl` (MPL-2.0, holder "AO Studio", matching `LICENSE`). `examples/`, `docs/`, and the tooling configs are excluded via `header_ignore`. Every `.go` file now carries the header, so the step is a no-op unless a file is added.

Generation runs `terraform fmt` but never `terraform validate`, which is why
`make validate-examples` (`scripts/validate-examples.sh`) exists separately: it
builds the provider into a throwaway filesystem mirror, synthesizes a
`required_providers` block for the example directories that have none, and runs
`terraform init` + `terraform validate` in each. That is the only check that
catches an example written against a schema the provider does not have — a
`settings { ... }` block where the schema is a `ListNestedAttribute`, an
attribute that does not exist, or a value a validator rejects. `dev_overrides`
cannot be used for this: it makes `terraform init` refuse to run.

`tfplugindocs` builds `docs/` from provider/resource/data-source `MarkdownDescription` strings plus the matching files under `examples/`. CI (`.github/workflows/test.yml`) fails the `generate` job if `make generate` produces a diff, so regenerate and commit whenever a schema, example, or guide changes.

`docs/` is fully generated and must never be hand-edited — tfplugindocs deletes and re-renders the whole directory on every run. All sixty-two pages are checked in: `index.md`, thirty-seven under `resources/`, twenty-two under `data-sources/`, and two under `guides/`.

Hand-written prose lives in `templates/`, which is the only part of the docs pipeline a human edits directly. `templates/guides/<name>.md.tmpl` renders to `docs/guides/<name>.md`; a `templates/` directory does not suppress the auto-generated resource and data-source pages, which are still built from the schemas into a temporary directory. Two guides exist: `getting-started` (credentials, installing the provider from the Registry and overriding it with a local build, first configuration, import IDs per resource) and `code-signing` (the `fastlane match` replacement, the state-vs-bundle distribution model, adopting a match certificate).

## Architecture

Two layers, strictly separated, plus a standalone CLI:

### `internal/apple/` — App Store Connect API client

- `client.go`: `Client` holds the JWT (ES256, `kid` header, 20-minute expiry) built from issuer ID + key ID + PKCS#8 private key. The token is minted once in `NewClient` and is immutable thereafter — a 20-minute life outlives any single Terraform command, so there is deliberately no refresh path and the signing key is not retained. `doRequest` decodes `models.ErrorResponse` into a formatted error, preferring Apple's own message; a 401 is reported as a credentials failure rather than retried, since re-signing with the same key yields an equivalent token.
- One file per Apple resource (`budleIds.go` — note the typo in the filename — `certificates.go`, `devices.go`, `merchantIds.go`, `passTypeIds.go`, `profiles.go`, `bundleIdCapabilities.go`, `apps.go`, `subscriptionGroups.go`, `subscriptionGroupLocalizations.go`, `subscriptions.go`, `subscriptionLocalizations.go`, `subscriptionPrices.go`, `subscriptionAvailabilities.go`, `subscriptionPromotionalOffers.go`, `subscriptionIntroductoryOffers.go`, `winBackOffers.go`, `inAppPurchases.go`, `inAppPurchaseVersions.go`, `inAppPurchaseLocalizations.go`, `inAppPurchasePrices.go`, `inAppPurchaseAvailabilities.go`, `territories.go`, `appInfos.go`, `appInfoLocalizations.go`, `appStoreVersions.go`, `appStoreVersionLocalizations.go`, `appStoreReviewDetails.go`, `ageRatingDeclarations.go`, `appPrices.go`, `appAvailabilities.go`, `builds.go`, `betaGroups.go`, `betaAppLocalizations.go`, `betaBuildLocalizations.go`, `betaAppReviewDetails.go`, `users.go`, `userInvitations.go`). Each exposes plain `Get*/Create*/Update*/Delete*` methods on `*Client` returning `models.*` structs. No Terraform types here.
- `models/`: JSON wire types. All responses go through the generics in `common.go`: `Response[T]`, `ListResponse[T]`, `Request[T]`.
- `pagination.go`: `getAllPages[T]` walks every page of a collection, following the absolute `links.next` URL until it is empty and refusing links that point off-host. All seven `Get*s()` functions go through it — reading only the first page silently truncates results. A `pageSize` of `noPageSize` (0) omits the `limit` parameter entirely, which the `bundleIdCapabilities` relationship requires: it rejects `limit` with a 400. `getAllPagesQuery` is the same walk with extra query parameters applied to the first request only — Apple's `links.next` already carries them forward, so re-appending would duplicate them. The subscription and in-app purchase collections need it: a price is unreadable without its price point included, and either price point catalogue is unusable without `filter[territory]`.
- Apple's API has no server-side filtering here — "get by identifier/name" helpers (e.g. `GetPassTypeIDByIdentifier`, `GetProfileByName`) list a full collection and scan client-side, so they depend on pagination being complete. The App Store Connect endpoints are the exception: `/v1/apps` supports `filter[bundleId]`, the price point catalogue supports `filter[territory]`, and `/v1/builds` supports `filter[version]` (the build number), `filter[preReleaseVersion.version]` (its train) and `filter[preReleaseVersion.platform]` — all used server-side because the collections are too large to pull whole. `GetBuilds` is the one by-identifier helper in the provider that is a single request rather than a full-collection scan, and it deliberately omits a `processingState` filter: see `apple_app_store_version` below.

### `internal/provider/` — Terraform layer

`provider.go` defines the `apple` provider: credential schema with regex validators (issuer ID = UUID, api_key = 10 chars `[A-Z0-9]`, private_key = PEM block), reads env vars **first** and lets explicit config override them, builds the `*apple.Client`, and hands it to every resource/data source via `resp.ResourceData` / `resp.DataSourceData`.

The optional `scope` attribute restricts the JWT to named operations. It must be left unset rather than configured as an empty list: `createToken` omits the claim unless it is populated, because App Store Connect rejects a token carrying an empty or null `scope` with a 400 `ENTITY_INVALID` titled "JSON processing failed" — an error about the token that reads like a complaint about the request body, and one that broke every API call the provider made.

Env vars: `APPLE_APP_STORE_CONNECT_ISSUER_ID`, `APPLE_APP_STORE_CONNECT_API_KEY`, `APPLE_APP_STORE_CONNECT_PRIVATE_KEY`.

Each domain lives in its own package (`bundle`, `certificate`, `device`, `merchant`, `passtypeid`, `profile`, `subscription`, `inapppurchase`, `app`, `beta`, `user`) following a consistent four-file convention:

| File | Contents |
|---|---|
| `models.go` | `tfsdk`-tagged state structs + exported `Get*Validator()` helpers shared by resource and data source |
| `resource.go` | CRUD + `Configure` + `ImportState` |
| `data_source.go` | plural listing data source: filter attrs, `limit`/`sort_by`/`sort_order`, computed `total_count`/`filtered_count` |
| `filters.go` | pure `Filter*` / `Sort*` / `Limit*` functions over `[]models.*`, applied in-memory after fetching |

`bundle` additionally carries `capability_*.go` for the `apple_bundle_id_capability` resource/data source, where `capability_models.go` holds `SettingsFromAPI`/`SettingsToAPI` and `OptionsFromAPI`/`OptionsToAPI` to convert nested capability settings between API structs and `types.List`.

`subscription` carries ten resources rather than one, so it prefixes by kind
the way `bundle` does: `group_*.go` (which covers both the group and its
localization), `localization_*.go`, `price_resource.go`,
`price_schedule_resource.go`, `availability_resource.go`,
`win_back_offer_*.go` (resource, listing data source, and its own
`win_back_offer_models.go`, kept out of `models.go`),
`introductory_offer_*.go` (the resource, its listing data source, and the
models, helpers and per-subscription write lock in
`introductory_offer_models.go`),
`price_point_data_source.go`, `price_point_equalizations_data_source.go` and
`promotional_offer_*.go` alongside the unprefixed `resource.go` /
`data_source.go` for `apple_subscription` itself. The promotional offer keeps
its state structs and helpers in `promotional_offer_models.go` rather than
`models.go`.
`territory` is a data source only — and the one App Store Connect listing that
takes no required scope argument, because `/v1/territories` is a real top-level
collection rather than a relationship hanging off an app. Its `ids` attribute is
a deliberate deviation: nothing else exposes a flat list of IDs alongside the
nested records, and it exists because `available_territories` on either
availability resource is otherwise only reachable through a `for` expression
every caller would have to repeat.

`app` was a data source only and is now the largest package in the provider: it
carries the nine resources that make up an app's App Store listing, prefixed by
kind the way `subscription` is — `settings_resource.go`, `info_resource.go`,
`info_localization_resource.go`, `age_rating_resource.go`, `version_resource.go`,
`version_localization_resource.go`, `review_detail_resource.go`,
`price_schedule_resource.go` and `availability_resource.go`, alongside
`category_data_source.go`, `price_point_data_source.go`,
`version_data_source.go` and the unprefixed `data_source.go` for `apple_apps`.
The shared state structs, validators and helpers live in `metadata_models.go`
and the pure filter functions in `metadata_filters.go`, keeping the original
`models.go` / `filters.go` for `apple_apps` itself. There is still no
`apple_app` resource, for the reason there never was one.

**Four things shape the whole `app` package**, and each of them is a place a
reasonable change goes wrong:

The metadata is split across two lifetimes. What describes the app — name,
subtitle, categories, privacy policy link, age rating — hangs off an `AppInfo`
and survives every release. What describes one release — description, keywords,
promotional text, copyright, release notes — hangs off an `AppStoreVersion` and
is replaced with it. EAS and fastlane present a single flat metadata file and
hide that; the provider cannot, because they are two Apple records with two
lifecycles. `examples/app-listing` keeps one `locales` map and fans it out to
both, which is the closest a configuration gets to the flat shape.

Most of these records already exist. Apple creates the `AppInfo`, the age rating
declaration and (for a released app) the price schedule and availability
alongside the app, and publishes no `POST` for any of them. `apple_app_settings`,
`apple_app_info` and `apple_app_age_rating_declaration` therefore **adopt on
create and drop state on destroy** — `Create` is a `PATCH`, and `Delete` warns.
Do not add a create path to these; there is nothing to create.

An `AppInfo` accepts a write only while it is being prepared.
`GetEditableAppInfo` picks the record whose state is one of
`models.EditableAppInfoStates` and errors with the states it did find otherwise.
Unlike `EnsureEditableInAppPurchaseVersion`, which creates a version when every
one is frozen, there is nothing to fall back on: Apple publishes no
`POST /v1/appInfos`. The three resources that hang off an app info
(`apple_app_info`, `apple_app_info_localization`,
`apple_app_age_rating_declaration`) all **re-resolve the editable record on every
read rather than following the ID in state**, because Apple issues a new
`AppInfo` — with new localization and declaration IDs under it — each version
cycle. Following the stored ID would eventually read a frozen record. Their
`Read` also treats "no editable app info" as *keep state as-is*, not as a
deleted resource: a review being open is not the same as the categories having
been removed.

**App Privacy is absent on purpose.** App Store Connect blocks a submission
until the data-collection questionnaire is answered, and Apple's published API
has no endpoint for it — the 4.4.1 OpenAPI spec contains no `appDataUsages`
path and no matching schema. Anything that automates it is using an
undocumented endpoint. It is declared per app rather than per version, so a
manual answer does not recur with each release; `examples/app-listing` reports
it, along with screenshots, the build upload and submission, through the
`remaining_manual_steps` output. Do not add a resource for it without first
confirming Apple has published one.

`beta` is TestFlight, and is the fastlane `pilot` replacement the way
`examples/signing` is `match` and `examples/app-listing` is `deliver`. Five
resources — `group_resource.go`, `tester_resource.go`,
`app_localization_resource.go`, `build_localization_resource.go` and
`app_review_detail_resource.go` — over `models.go`, with no data source and no
`filters.go`, because nothing here lists. **Four things shape it:**

TestFlight metadata splits across two lifetimes the same way App Store metadata
does, and for the same reason: `apple_beta_app_localization` hangs off the app
and survives every build, `apple_beta_build_localization` hangs off a build and
is replaced with it. Do not fold them together; they are two Apple records.

Internal and external groups are **not one flag**. `isInternalGroup` and
`hasAccessToAllBuilds` are absent from Apple's `BetaGroupUpdateRequest`, so both
are `RequiresReplace`, and the two kinds accept disjoint attribute sets — only an
external group can carry a public link. `ValidateConfig` rejects
`public_link_*` on an internal group at plan time rather than letting Apple
refuse it with a message that names the attribute and not the reason.

Membership is a **linkage, not a record**. `apple_beta_tester` is one tester in
one group, and the Apple record behind it belongs to the account rather than to
the group — one per email address, shared by every group and app it is in. So
`Delete` withdraws the membership through
`DELETE /v1/betaGroups/{id}/relationships/betaTesters` and never
`DELETE /v1/betaTesters/{id}`, which would remove the person from every app in
the account.

**Build assignment is still absent**, and so is beta review submission. The
first is real state and simply is not modelled yet
(`betaGroups/{id}/relationships/builds`); the second is an event, and falls
under the same rule `apple_app_store_version` submission does — a resource for
it would cancel a live review on destroy and resubmit on apply.
`has_access_to_all_builds` is what a group without build assignment uses.

`user` is Users and Access, and the only part of the provider that manages the
**team** rather than anything the team ships — `resource.go`,
`invitation_resource.go`, `data_source.go`, `filters.go` and `models.go`, with
no app ID anywhere in it. **Four things shape it:**

Membership splits across two Apple records with two lifetimes, the way App Store
and TestFlight metadata do, and for a sharper reason: a `UserInvitation` is an
offer, and Apple **destroys it the moment it is accepted**, issuing a `User`
with a different ID in its place. So an invitation that 404s has been accepted,
cancelled or has lapsed, and the 404 cannot say which. `Read` asks
`GetUserByUsername` before dropping anything: a member who now exists sets the
computed `accepted` and `user_id` and **keeps state as-is** — the
"keep state, do not report a deletion" rule the app info resources follow —
because planning a recreation would have Terraform re-invite somebody Apple says
is already on the team. Nothing else is refreshed in that branch: every
attribute of an invitation is `RequiresReplace`, so adopting a role changed
since through `apple_user` would plan exactly that recreation.

`apple_user` **adopts and never creates**, like `apple_app_settings` — Apple
publishes no `POST /v1/users` — but unlike every other adopting resource here it
has a **real `Delete`**, and that delete removes a person from the team. That
asymmetry is deliberate: leaving them in place would be the wrong answer to a
configuration that no longer names them. Do not turn it into a warn-and-drop.

Apple publishes **no `GET /v1/users/{id}`** either. The collection is the only
read, so `GetUser` scans it the way the Developer Portal by-identifier helpers
do, and both resources accept an email address as an import ID as readily as
Apple's own — nobody has the opaque one, and the request is the same either way.
`filter[username]` and `filter[email]` are sent where they exist, and the match
is made again in memory, so a filter Apple ignores costs a larger page rather
than the wrong person.

`visible_apps` is a **paginated collection, not a field**, so reading it is a
second request — and for a member whose `allAppsVisible` is true Apple answers
it with every app on the team. Both resources therefore refresh it **only when
state holds a list**, and the data source asks only for members reported as
restricted. Adopting it unasked would write the whole catalogue into state and
show a diff for something nobody configured. `ACCOUNT_HOLDER` is rejected in
`ValidateConfig` rather than at apply: Apple reports the role and refuses every
request that grants it, so a configuration carrying it could never apply.

`inapppurchase` is the same shape for one-time purchases: `localization_*.go`,
`price_schedule_resource.go`, `availability_resource.go` and
`price_point_data_source.go` alongside the unprefixed `resource.go` /
`data_source.go` for `apple_in_app_purchase`. It shares nothing with
`subscription` — Apple models the two as separate resources, and a price point
from one is meaningless to the other.

**App Store Connect resources differ from Developer Portal ones in two ways
that shape the whole `subscription` package.** They hang off an app record,
which Apple's API cannot create — its documentation says "Don't use this API to
create new apps; instead, create new apps on the App Store Connect website" —
so there is deliberately no `apple_app` resource, only the `apple_apps` data
source. And they nest: a group holds subscriptions, a subscription holds
localizations and prices, and Apple publishes **no top-level collection** for
any of them (`GET /v1/subscriptions` and `GET /v1/subscriptionGroups` are both
404), so the parent must be known before a child can be read. That is why every
listing data source takes a required scope argument, and why several import
forms are composite.

In-app purchases take both of those further. `GET /v2/inAppPurchases` is a 404
too — a purchase is reachable only through `/v1/apps/{id}/inAppPurchasesV2` —
and the purchase record has **no app relationship at all**, so the parent cannot
even be read back from a child that was found. Their metadata nests one level
deeper as well: localizations hang off an in-app purchase version rather than
the purchase.

**Adding a resource or data source requires registering the constructor in both `DataSources()` and `Resources()` in `internal/provider/provider.go`** — nothing is discovered automatically.

### `examples/signing` — the fastlane match replacement

A runnable module, not a documentation snippet. It manages the App ID, its
capabilities, the devices, the certificates and one profile per distribution
method.

**The signing key never enters Terraform.** The user generates the key and the
CSR locally and supplies only `var.csr_contents`; a CSR is a public key plus a
signature over it. That single decision is what makes the state ordinary: a
CSR, an issued certificate and a provisioning profile are all public documents,
so nothing in the state or the outputs is secret, and the module needs no
secret-store dependency, no encrypted bundle, and no companion CLI. The last
mile — a `.p12` assembled from the certificate and the user's key, plus
profiles copied where Xcode reads them — is documented in
`examples/signing/README.md` and `templates/guides/code-signing.md.tmpl` rather
than automated.

There was previously a `cmd/applesign` CLI and a `local.signing_bundle` JSON
document carrying private keys between them. Both are gone; do not reintroduce
either without also reintroducing the reason they existed.

Certificates reach the outputs through `local.certificates`, which merges the
ones the module issues with ones adopted from an existing serial via
`data.apple_certificates` — adoption exists because importing an
`apple_certificate` reissues it (`csr_content` forces replacement and Apple does
not reliably return the CSR), and issuing a replacement revokes the original. An
adopted role needs no CSR.

Two things there are easy to get wrong:

- `local.certificates` and the `profiles` output call `nonsensitive()` on
  `certificate_content` and `profile_content`. The provider marks both
  `Sensitive`, but they are public documents, and leaving them wrapped would
  force every output to be sensitive and redact values that are safe to print.
  `nonsensitive()` **errors on a value that is not sensitive**, so un-marking
  either attribute in the provider schema breaks this module.
- `required_version` is `>= 1.9` because `var.csr_contents` uses a `validation`
  block that references `var.adopt_certificate_serials` — cross-variable
  validation landed in 1.9. That check is what enforces "the distribution role
  needs either a CSR or a serial", which nothing else can express: a missing
  role would otherwise surface as an invalid map index deep in a profile.

Note that `terraform validate` does not evaluate variable validation for a
variable left at its default, so `make validate-examples` passes without any
CSR configured. The rules only fire at plan time.

### `examples/app-listing` — the metadata-file replacement

A runnable module, not a snippet, and the counterpart to `examples/signing`. It
manages an app's whole App Store listing from one `terraform.tfvars`: content
rights, categories, age rating, the localized name and product page, the
release, App Review information, the price and the storefronts.

It exists because the obvious question — "can this replace my EAS
`store.config.json` / fastlane `Deliverfile`?" — has a two-part answer, and the
module is where both parts are demonstrated rather than argued. It keeps a
single `locales` map and fans it out to both
`apple_app_info_localization` and `apple_app_store_version_localization`, which
is the closest a configuration gets to the flat shape those files use. And its
`remaining_manual_steps` output names what no configuration can do: App Privacy,
screenshots, the build upload, export compliance and submission. Attaching the
build is not on that list — `build_number` handles it — but uploading the binary
is, because Apple publishes no upload endpoint.

Two things there are easy to get wrong:

- The `advisory` variable defaults every frequency answer to `NONE` and every
  boolean to `false`. That is right for an app with no mature content and wrong
  to leave unread, and the variable description says so. It is not the same as
  omitting them — the resource omits an unset attribute from the `PATCH`, which
  leaves Apple's existing answer alone.
- `review.demo_account_password` is in state in plain text. The variable is
  marked `sensitive`, which keeps it out of console output and not out of state,
  and a `validation` block enforces that `demo_required` implies both a name and
  a password, since Apple rejects the review detail otherwise.

Unlike `examples/signing`, `required_version` is only `>= 1.3` — the module uses
`optional()` with defaults in object types, which landed there, but no
cross-variable validation.

## Conventions to follow

- Data sources deliberately expose **no `last_updated` attribute**. A data source is re-read on every plan and the framework gives it no prior state, so such a value can only ever be "now" — it churns every run and propagates a perpetual diff to anything referencing it. Do not reintroduce it.

- Resource type names come from `req.ProviderTypeName + "_bundle_id"` in `Metadata`; data sources use the plural form (`_bundle_ids`).
- Every resource/data source declares its interface assertions (`var _ resource.ResourceWithImportState = &fooResource{}`) and a `Configure` that type-asserts `req.ProviderData.(*apple.Client)`, erroring with the received type on mismatch.
- Immutable Apple attributes get `stringplanmodifier.RequiresReplace()`; Apple-computed attributes get `stringplanmodifier.UseStateForUnknown()`.
- On an Optional+Computed attribute that is also immutable, `UseStateForUnknown()` comes **before** `RequiresReplace()`. The framework marks an omitted computed value unknown whenever anything else about the resource changes, and `RequiresReplace` compares the planned value with state, where unknown never equals a known value: in the other order, renaming an `apple_beta_group` without restating `is_internal_group` planned its replacement. `TestOmittedComputedValueNeverForcesReplacement` (`internal/provider/plan_modifier_order_test.go`) plans every resource's top-level Optional+Computed attributes this way and fails on the wrong order.
- `MarkdownDescription` (not `Description`) everywhere — it is the source for generated docs, so enumerate valid enum values in the string.
- **A step that fails after a create succeeded is a warning, not an error, when replacement cannot repair it.** Terraform core taints any object whose create returns an error (`maybeTainted` in `internal/terraform/node_resource_apply_instance.go`) and plans its replacement. For a record Apple cannot delete, or whose identifier it never releases, that replacement is a second record or a refused create. Return the planned values for everything known -- core rejects a create whose state differs from a known planned value ("Provider produced inconsistent result after apply", from `AssertObjectCompatible`), which is an error and taints it all the same -- then warn. The next refresh reads what Apple really holds, and the next plan finishes the job as an update; `Read` must therefore refresh whatever the failed step was to set. The offer code resources do this for a deactivation that fails after create. Where replacement is harmless, an error is fine: an editable `apple_app_store_version` can be deleted and made again.
- Error handling is deliberately verbose: `strings.Contains` on the API error to map "already exists" / "not found" / 404 into specific, actionable diagnostics rather than surfacing raw HTTP errors. Match this when adding operations.
- Logging uses `tflog` with `tflog.SetField` to attach structured context; never log credential values.
- `ImportState` is hand-written per resource and generally accepts either Apple's opaque ID or a human identifier (bundle identifier, UDID, certificate serial/display name, profile name), disambiguating by string shape.

## Resource-specific behavior worth knowing

- **`apple_certificate`**: immutable — every attribute describing the certificate is `RequiresReplace`, and Delete revokes it. `certificate_content` is `Sensitive`. Optional `early_renewal_hours` triggers replacement before expiry: `ModifyPlan` compares prior state's `expiration_date` against the window, flips the computed `ready_for_renewal` to true, marks the Apple-issued attributes unknown, and reports `ready_for_renewal` as the path forcing replacement. `Create`/`Read`/`ImportState` all record `ready_for_renewal` as false so the flip registers as a change. `Update` exists solely to accept a changed `early_renewal_hours` in place — it copies the issued certificate forward untouched. `Read` never refreshes `csr_content` or the `requester_*` fields from the response: Apple returns `csrContent` as null and omits the requester fields, so assigning them would blank values only the configuration and the create response hold — and since `csr_content` is `RequiresReplace`, a blanked CSR makes the next plan revoke the certificate and issue a new one.
- **`apple_bundle_id`**: `platform` accepts `IOS`, `MAC_OS` and `UNIVERSAL` — Apple rejects `TV_OS` and `WATCH_OS`, and tvOS and watchOS App IDs are created as `UNIVERSAL`. Apple then stores *every* Bundle ID as `UNIVERSAL` regardless of what was sent, so `Create` and `Read` keep the configured value rather than the reported one: refreshing it would rewrite a configured `IOS` to `UNIVERSAL`, and `platform` is `RequiresReplace`, so the next plan would destroy the Bundle ID and take its capabilities with it. `Read` only adopts Apple's value when state has none (an import) or when Apple reports something that was never a valid input.
- **`apple_merchant_id`**: Apple names the human-readable field `name` on `merchantIds`, not `displayName`; the Terraform attribute stays `display_name` and only the wire tag differs. Import distinguishes the two accepted forms by the `merchant.` prefix — Apple's own IDs are 10-character alphanumerics like `4B62XU6945`, not UUIDs.
- **`apple_device`**: the Apple API cannot delete devices, so `Delete` disables the device (`PATCH` with `status: DISABLED`) and lets Terraform drop it from state, emitting a warning. A 404 while disabling is treated as already-gone so destroy still succeeds.
- **`apple_bundle_id_capability`**: Apple exposes no `GET` for a single capability (403 `does not allow 'GET_INSTANCE'`), so every read lists the parent Bundle ID's collection and scans — which is why the parent has to be known. Import accepts `<bundle_id>/<capability_id>`; a bare capability ID recovers the parent from the ID itself, which Apple composes as `<bundleID>_<CAPABILITY_TYPE>`, and confirms it against the collection before writing state. The lookup through the parent *is* the ownership check. Capability writes are serialized per Bundle ID (`capability_serialize.go`) because Apple applies them as an unlocked read-modify-write over the whole set: two concurrent creates both return 201 and one silently vanishes. The resource models only what Apple accepts as input — `key` plus `value` or `options` — since Apple has no scalar `value` property (`value` is sent as a one-element options list) and its display metadata, exposed as `Optional+Computed` inside a list, made every plan non-empty.

  The resource and the data source therefore describe a setting differently on purpose: `name`, `visible`, `min_count` and the per-option labels stay on the data source (`capability_data_source.go`), which only reports, and are absent from the resource, which only writes. Dropping them from the resource narrowed a published schema, which is normally breaking — it was safe here solely because no configuration could have been relying on them: every capability operation failed before this, so the resource had never created anything. That escape hatch does not generalise. Removing an attribute from a resource that works needs a deprecation cycle instead.
- **`apple_subscription_group`**: `app_id` is write-once and unreadable. Apple's update request has no `app` member and `GET /v1/subscriptionGroups/{id}` accepts no `app` value for its `include` parameter, so the owning app can never be refreshed — `Read` keeps the configured value, and `ImportState` accepts only the composite `<app_id>/<group_id>` form, confirming ownership against the app's collection before writing state. `reference_name` is the only mutable attribute; it is internal to App Store Connect, and the customer-facing name lives on `apple_subscription_group_localization`.
- **`apple_subscription_group_localization`**: the customer-facing half of a group — the heading above the list of plans, plus an optional `custom_app_name` overriding the app name shown beside it. It is the **one place in the group hierarchy whose parent can be read back**: `GET /v1/subscriptionGroupLocalizations/{id}` accepts `include=subscriptionGroup`, where the group's own `GET` accepts no `app`. That asymmetry is why this resource imports from a bare Apple ID and `apple_subscription_group` refuses to. `locale` is `RequiresReplace` — it identifies the record and is absent from Apple's update request, the same shape as `apple_subscription_localization`; `name` and `custom_app_name` are the only two members of that request. `Delete` tolerates a `last localization` 409 defensively, in case Apple applies to a group the rule an in-app purchase version is under: erroring there would fail the whole destroy and leave the group behind, and the record goes away with the group anyway. That branch is unverified against the live API — Apple's documentation does not state the rule for groups.
- **`apple_subscription`**: `product_id` is `RequiresReplace` because Apple's `SubscriptionUpdateRequest` has no `productId` member — and replacing it is expensive in a way Terraform cannot express, because **Apple never releases a product identifier**, not even one belonging to a subscription deleted before it was ever approved. `group_id` is likewise immutable: a subscription cannot move between groups. `state` is Apple's and always starts at `MISSING_METADATA`; it only leaves once a localization *and* a price exist, which is why those are separate resources rather than optional attributes. `Read` and `ImportState` rely on `GetSubscription` requesting `include=group`, without which Apple reports the relationship as links alone and an import would leave `group_id` null.
- **`apple_subscription_localization`**: `locale` is `RequiresReplace` — it identifies the record and is absent from Apple's update request. This is the customer-facing name and description; the `name` on `apple_subscription` is an internal reference name. Import accepts a bare localization ID (the client asks for `include=subscription`) or the composite `<subscription_id>/<localization_id>`, which additionally checks ownership.
- **`apple_subscription_price`**: every attribute is `RequiresReplace` and `Update` exists only to report that it was reached, because Apple publishes no `PATCH /v1/subscriptionPrices` — a price change is a new record plus a deletion. There is also no `GET` for a single price, so `Read` lists the subscription's price collection and scans it, the same shape `apple_bundle_id_capability` is forced into; import is therefore composite. A price is never a number: `price_point_id` references Apple's catalogue, read through `apple_subscription_price_points`, and a price point ID encodes the subscription it belongs to, so it cannot be reused across subscriptions. `preserve_current_price` is an instruction Apple does not report back — only its outcome, through the computed `preserved` — so import leaves it null and `ImportStateVerifyIgnore` covers it.
  **A price requires an `apple_subscription_availability` to exist first**, and nothing about the failure says so: Apple answers `POST /v1/subscriptionPrices` with a 409 `There is a problem with the request entity - An error occurred while processing the pricing information`, naming neither availability nor the territory. That message cost a full afternoon of eliminating hypotheses — the price point, an empty `attributes` member, a missing `territory` relationship, and app-level pricing and availability were all ruled out by experiment before the subscription's own availability turned out to be the prerequisite. `Create` therefore special-cases the string `processing the pricing information` and names the missing availability in the diagnostic; do not fold that case back into the default branch. Nothing in a price references an availability, so a configuration has to declare `depends_on` to get the ordering — both the example and the acceptance test do.
  Two things follow from a price that actually exists, both of which only became reachable once availability unblocked creation. **`territory_id` is `Optional+Computed`, not merely Optional:** Apple reports the territory of every price whether or not one was sent, so a plain Optional attribute had `Read` writing `USA` into state against a configuration holding null — and because the attribute is `RequiresReplace`, the next plan destroyed the price to remove it. Making it computed then exposed the other half: Apple omits the territory from the create response — only a read carries one, since the read asks for `include=territory` and a `POST` takes no `include` — so `Create` re-reads the price to resolve the unknown, and falls back to null rather than leaving an unknown the framework would reject. **`Delete` tolerates Apple's 409 `Only future price changes can be deleted`:** a live price is not a record that can be withdrawn, it is what the subscription currently costs, and the only way past it is a later price that supersedes it. Warn and drop state, the way `apple_device` does; deleting the subscription removes its prices anyway.
- **`apple_subscription_price_schedule`**: the bulk counterpart to
  `apple_subscription_price`, and the resource the equalization fan-out is
  actually for. `POST /v1/subscriptionPrices` takes one price point in one
  territory, so pricing all 175 storefronts through the singular resource is 175
  resources and 175 requests on apply — and, because Apple publishes no `GET`
  for a single price, a refresh that lists the subscription's whole price
  collection once *per resource*. Apple's own forum guidance says to automate
  the loop, which is why it reads as though no bulk form exists. **It does, and
  it is not on `subscriptionPrices`:** `SubscriptionUpdateRequest` carries a
  `prices` relationship (alongside `introductoryOffers` and
  `promotionalOffers`) and an `included` member accepting
  `SubscriptionPriceInlineCreate`, so the write is one
  `PATCH /v1/subscriptions/{id}` with each price inline under a `${price0}`
  placeholder — the same JSON:API shape
  `apple_in_app_purchase_price_schedule` and `apple_app_price_schedule` use.
  Five things follow:
  - It is a **separate request type** from `SubscriptionUpdateRequest`, not a
    field on it, because that one sends a non-optional `attributes` member. An
    empty `"attributes":{}` is what Apple answers with the 409 about
    "processing the pricing information" — the same trap
    `CreateSubscriptionPrice` guards against, now factored into
    `priceAttributesOrNil`.
  - There is **no schedule record at Apple**. Unlike an in-app purchase, a
    subscription has no `iapPriceSchedule` equivalent: the prices are the
    subscription's. So `id` mirrors `subscription_id`, the import ID is the
    subscription ID, and none of the `ModifyPlan`-marks-`id`-unknown machinery
    the other two price schedules need applies here.
  - The nested price carries **no computed attributes** — no per-price `id`, no
    `preserved`. A computed attribute inside a set makes the element unknown at
    plan time, which leaves Terraform unable to match a planned element against
    the one in state. `territory_id` is therefore plain `Optional` here, where
    on `apple_subscription_price` it had to become `Optional+Computed`; the
    same problem is solved by `reconcileSchedulePrices` not refreshing a price
    already in state, the way `apple_in_app_purchase_price_schedule` keeps
    configured dates.
  - A write **replaces the subscription's manual price set**, so do not point
    this and `apple_subscription_price` at the same subscription — a price only
    the other resource knows about would come off behind its back. Removal is
    still subject to Apple's rule that only future price changes can be
    deleted.
  - The same prerequisites as the singular resource: an
    `apple_subscription_availability` has to exist first and the 409 names
    neither it nor a territory, so the `processing the pricing information`
    special case is carried over. Apple additionally refuses the write while
    the subscription is in review.

- **`apple_subscription_price_point_equalizations`**: the data source that makes
  pricing every territory possible, and the reason `apple_subscription_price_points`
  alone is not enough. The catalogue read answers *what may this subscription
  cost in these territories*, which leaves a configuration pricing 175
  storefronts to decide each one: there is no `customer_price` to filter on,
  because 9.99 in the USA is neither 9.99 nor a round number anywhere else.
  `GET /v1/subscriptionPricePoints/{id}/equalizations` is Apple's own answer —
  the mapping App Store Connect's price matrix is built from — so the provider
  reads it rather than reimplementing it. `price_point_ids` (territory code →
  price point ID) is the second deliberate deviation of the `apple_territories`
  `ids` kind, and exists for the same reason: the fan-out is a `for_each` over
  that map, where reaching the same IDs through `price_points` needs a `flatten`
  and a one-element index expression in every configuration. It is built by
  `PricePointIDsByTerritory` in `filters.go`, which skips a point with no
  territory linkage rather than keying it under the empty string, and depends on
  `include=territory` exactly as the catalogue read does. An equalized point
  belongs to the same subscription the base point does — a price point ID
  encodes its subscription — so the read is per subscription and its results are
  not portable to another. Whether Apple returns the base territory's own point
  is Apple's call, which is why the examples `merge` the base point in rather
  than assume. The same endpoint exists for the other two catalogues
  (`/v1/inAppPurchasePricePoints/{id}/equalizations`,
  `/v1/appPricePoints/{id}/equalizations`) and is not implemented; those
  catalogues share nothing with this one.
- **`apple_subscription_introductory_offer`**: one offer — a free trial or a
  discounted first period — across **a set of territories**, where Apple keeps
  **one record per territory**: a one-week trial everywhere is 175
  `subscriptionIntroductoryOffers` with identical terms. A resource per record
  would be 175 resources for one decision, so the resource describes the offer
  once and owns a record in each territory it names, reported through the
  computed `offer_ids` (territory → record). `id` mirrors `subscription_id`, as on
  the price schedule. Five things follow:
  - The territories live in **exactly one of two attributes**, chosen by whether
    the offer has a price: `territory_ids` (a set) for `FREE_TRIAL`, which has
    none, and `price_point_ids` (territory → price point) for `PAY_AS_YOU_GO` and
    `PAY_UP_FRONT` — the map `apple_subscription_price_point_equalizations`
    already produces, so the fan-out needs no `for` expression. Apple's spec
    marks the price point optional and states no rule per mode; `ValidateConfig`
    rejects a price on a free trial and its absence on a paid offer, because a
    paid offer's price is the offer. **The territory is always sent**, though
    Apple marks it optional too: a price point encodes its territory, a free
    trial has nothing to infer one from, and what a record with no territory
    means is undocumented — so the provider never creates one.
  - **Creation is one `PATCH /v1/subscriptions/{id}`**, not a `POST` per
    territory: `SubscriptionUpdateRequest` carries an `introductoryOffers`
    relationship and `included` accepts `SubscriptionIntroductoryOfferInlineCreate`,
    the shape the price schedule found for prices. Apple commits it whole, which
    matters more here than for prices — a `POST` loop that fails part-way leaves
    records state never saw, and the next apply collides with them because a
    territory holds one introductory offer at a time. **The relationship repeats
    every offer the subscription already has**, read first through
    `/relationships/introductoryOffers`: Apple's spec does not say whether a
    to-many member in a `PATCH` replaces or appends, and JSON:API says replaces,
    so sending only the new ones risks deleting every other offer on the
    subscription — another resource's, or one made in App Store Connect. Naming
    them all is right under either reading. That read-modify-write is why writes
    are **serialized per subscription** (`lockSubscriptionIntroductoryOffers`,
    the `capability_serialize.go` pattern), and the same read identifies the new
    records afterwards: the `PATCH` answers with the subscription, so the offers
    are listed again and the IDs that were not there before are this call's.
  - **There is no `GET /v1/subscriptionIntroductoryOffers/{id}`** — only `PATCH`
    and `DELETE` — so `Read` lists the subscription's collection (filtered to
    the territories in state) and matches **by record ID, never by territory**:
    a territory can also hold an ended offer or one scheduled after this one,
    and neither is this resource's. A record that has gone takes its territory
    out of state so the next plan recreates it. Only `end_date` is compared, and
    only when one is configured; `start_date` and `target_subscription_plan_type`
    are `RequiresReplace` and never refreshed, the `apple_app_settings` rule,
    since Apple may report a value the configuration never set.
  - **Apple's update request carries `endDate` alone**, so every other attribute
    is `RequiresReplace`, and `end_date` is a `PATCH` per record — sent with an
    explicit `"endDate": null` to remove it (no `omitempty`; omitting the member
    would leave the old date). The territory set changes in place: deletions
    first, so a territory whose price point changed has no overlapping offer
    when it is created again, then end dates, then one bulk create. `ModifyPlan`
    marks `offer_ids` unknown only when the territory set changes. After any
    failure the state written is what Apple holds.
  - **Destroy deletes every record** with `DELETE /v1/subscriptionIntroductoryOffers/{id}`
    (404 counts as gone). Apple also publishes `DELETE
    /v1/subscriptions/{id}/relationships/introductoryOffers`, one request for
    many, but whether removing a linkage deletes the record is undocumented, so
    it is not used. Whether Apple deletes an offer that is already running is
    unverified: a refusal is an error naming the territories and leaves those
    records in state, and the diagnostic suggests `end_date` instead.
  Import is `<subscription_id>/<offer_id>` with any one territory's record, and
  adopts every record on the subscription with the same terms — what App Store
  Connect's UI creates for one offer. `apple_subscription_introductory_offers`
  lists the raw records, one per territory, which is how those IDs are found.
- **`apple_subscription_availability`**: the same singular, `POST`-replaces, no-`PATCH`, no-`DELETE` shape as `apple_in_app_purchase_availability`, and the same consequences — `Update` posts again, `Delete` warns and drops state only, and `ModifyPlan` marks `id` unknown when anything else changes because Apple issues a new record ID on every `POST`. It is a separate Apple resource from the in-app purchase one (`/v1/subscriptionAvailabilities` against `/v1/inAppPurchaseAvailabilities`) and the two share no code, the same way the two price catalogues do not. The record carries only `availableInNewTerritories`; the territory list is a paginated collection under `/v1/subscriptionAvailabilities/{id}/availableTerritories`, so `Read` takes two requests. Import ID is the subscription ID. Its real significance is ordering: it gates pricing, so a subscription cannot leave `MISSING_METADATA` without it. `available_territories` is required, and Apple's default of every territory applies only to a product whose availability has never been set — once this resource exists, selling everywhere means naming everywhere, which is what `data.apple_territories.all.ids` is for.
- **`apple_subscription_promotional_offer`**: a discount the app, not the App
  Store, presents — the app requests it by `offer_code` with a signature its
  server computes, so the code is the one value app and configuration must
  agree on. Apple's `SubscriptionPromotionalOfferUpdateRequest` has **no
  attributes member at all**, only the `prices` relationship and its `included`
  inline prices, so every attribute is `RequiresReplace` and `prices` is the
  only thing `Update` sends. Create and update both carry the prices inline
  under `${price0}` placeholders, the shape the price schedules use, but the
  inline `SubscriptionPromotionalOfferPriceInlineCreate` names only `territory`
  and `subscriptionPricePoint` — it does not repeat the parent the way
  `SubscriptionPriceInlineCreate` does. Four things follow:
  - **Prices are read from their own collection**, never `include=prices` on
    the offer: Apple caps that include at `limit[prices]=50`, fewer than the
    storefronts an offer can be priced in, so `GetSubscriptionPromotionalOfferPrices`
    walks `/v1/subscriptionPromotionalOffers/{id}/prices` with
    `include=territory,subscriptionPricePoint`. `Read` is therefore two requests.
  - `prices` is a `types.Set` of `{territory_id, price_point_id}` with no
    computed member, for the reason the price schedule's has none.
    `reconcilePromotionalOfferPrices` matches by **territory** — an offer has one
    price per territory — keeps a state entry as it is, takes Apple's price
    point when it differs (drift), adds territories only Apple reports and drops
    ones it no longer does. A state entry with no price point is kept whatever
    Apple reports for it.
  - **Apple requires the prices relationship on every create, a free trial
    included.** Both inline relationships are optional in Apple's schema, so a
    `FREE_TRIAL` territory sends a territory and no price point;
    `ValidateConfig` demands a price point in every territory for the two paid
    modes. Whether Apple accepts the territory-only shape is the first thing an
    acceptance run should confirm (`TestAccSubscriptionPromotionalOfferResource_freeTrial`).
  - **The offer code's lifetime is undocumented.** The spec types `offerCode`
    as a bare string and says nothing about uniqueness or whether a deleted
    offer's code is released, so the schema tells users to treat a code as
    spent and the acceptance tests randomise it, the way product identifiers
    are. `target_subscription_plan_type` is `Optional+Computed` with
    `UseStateForUnknown` ahead of `RequiresReplace`, so a value Apple reports
    for an offer that never named one does not plan a replacement.
  Apple publishes a real `DELETE`, so destroy deletes. Import accepts the bare
  offer ID (the `GET` takes `include=subscription`) or
  `<subscription_id>/<offer_code>` — the form a hand-made offer is known by,
  found by scanning `/v1/subscriptions/{id}/promotionalOffers`, which is also
  what `apple_subscription_promotional_offers` lists for the same reason.
- **`apple_subscription_win_back_offer`**: an offer the App Store shows to customers whose subscription has lapsed, on `/v1/winBackOffers`, with a real create, `PATCH` and `DELETE`. **Apple's OpenAPI schema for the inline price is empty** — `WinBackOfferPriceInlineCreate` in the 4.5.1 spec declares only `type` and `id`, no relationships, which would be a price with no price point. Apple's own "Create a Win-Back Offer" example sends a `subscriptionPricePoint` relationship on each inline price and nothing else, and the read-side `WinBackOfferPrice` carries exactly `territory` and `subscriptionPricePoint`, so the client follows the example: the price point always, the territory only when configured, and — unlike `SubscriptionPriceInlineCreate` — no relationship repeating the parent. Confirm this first on an acceptance run. Five more things follow from Apple's shapes:
  - **The update request has no prices** (and no reference name, offer ID, duration, mode, period count or plan type), and App Store Connect itself refuses a price change after creation, so `prices` and all six are `RequiresReplace` and the schema says so. What `PATCH` does accept — the three eligibility rules, the dates, `priority`, `promotion_intent` — is sent **only when it changed**, built by `winBackOfferUpdate` from plan against state: Apple refuses some changes once an offer is live, and resending an unchanged start date could fail a change it would otherwise take. A removed `end_date` or wait goes as an explicit `null` (`WinBackOfferUpdateAttributes.MarshalJSON`), because an omitted member keeps Apple's value.
  - **A replacement needs a new `offer_id`.** Apple says it must not have *already been used* for another offer in the subscription group, and its API docs add promotional offers, offer codes and introductory offers to the namespace — so it is treated as reserved forever, and the acceptance test randomises it the way product IDs are.
  - **An offer never names its subscription.** `WinBackOffer` has no subscription relationship and its `GET` includes only `prices`, so `subscription_id` is kept as configured and import is `<subscription_id>/<offer>`, where `<offer>` is Apple's ID or the `offer_id`, matched while listing the subscription's offers — which is also the ownership check. That listing is what `apple_subscription_win_back_offers` exposes, and it earns its place for the same reason: it is the only way to find an offer made in App Store Connect.
  - **`reconcileWinBackOfferPrices` never adopts a price over configuration.** A configured price is kept exactly as written while Apple still reports its price point (Apple reports a territory whether or not one was sent), a missing one is dropped, and an extra one is adopted only when state holds no prices — an import. Adopting an extra on an ordinary refresh would plan the destruction of a live offer, since the set is `RequiresReplace`.
  - `target_subscription_plan_type` is `Optional+Computed+RequiresReplace`, and **`UseStateForUnknown` has to come before `RequiresReplace`** in its plan modifiers: an omitted value plans as unknown, and `RequiresReplace` reading unknown-against-state as a change made every in-place update replace the offer. `TestTargetPlanTypeOmittedDoesNotReplace` runs the schema's own modifiers to pin it. The integer limits (paid tenure 1–24 or 36/48/60 months, lapsed 1–24, wait 2–24) and the 350-per-subscription / 5-live-per-storefront caps come from App Store Connect Help, not the spec, which types every member as a bare integer; the cross-attribute checks (window order, end after start) are in `ValidateConfig`. Apple creates offers only on a subscription App Review has approved, so the acceptance test needs `APPLE_TEST_APPROVED_SUBSCRIPTION_ID`.
- **Customer-facing text is validated in characters, not bytes.** `GetNameValidator` and `GetDescriptionValidator` in `subscription` and `inapppurchase`, `GetAppNameValidator`, `GetSubtitleValidator`, `GetVersionDescriptionValidator`, `GetPromotionalTextValidator` and `GetWhatsNewValidator` in `app`, and `GetBetaGroupNameValidator`, `GetBetaDescriptionValidator`, `GetWhatsNewValidator` and `GetReviewNotesValidator` in `beta`, all use `stringvalidator.UTF8LengthBetween`, not `LengthBetween`. Apple's 30- and 45-character limits are character counts, while `LengthBetween` counts bytes, which rejected a legal 27-character Arabic description at "49". Any new customer-visible string field must follow this.
- **`apple_in_app_purchase`**: the one-time purchase side of the App Store — `CONSUMABLE`, `NON_CONSUMABLE`, `NON_RENEWING_SUBSCRIPTION` — on `/v2/inAppPurchases`, and a different Apple resource from `apple_subscription`. `app_id` is worse than write-once: `InAppPurchaseV2` has **no app relationship at all** and no `include` produces one, so the app can never be read back, `Read` keeps the configured value, and `ImportState` accepts only `<app_id>/<in_app_purchase_id>` — confirming ownership by listing the app's collection, which is the only check the API offers. `product_id` and `in_app_purchase_type` are `RequiresReplace` because Apple's update request has neither member, and replacing either is expensive: Apple never releases a product identifier. `Read` never refreshes `app_id`.
- **`apple_in_app_purchase_localization`**: localizations hang off an **in-app purchase version**, not the purchase. Apple moved them there in App Store Connect API 4.4.1 and deprecated the v1 endpoints that hid it, so the provider writes to `/v2/inAppPurchaseLocalizations` against a version resolved by `EnsureEditableInAppPurchaseVersion`: it picks the highest-numbered version in `PREPARE_FOR_SUBMISSION`, `DEVELOPER_REJECTED` or `REJECTED`, and creates one when every version is in review or approved — which is what App Store Connect's UI does when you edit approved metadata. Versions have no `DELETE`, so one created that way outlives the localization. The resolved version is reported as the computed `version_id`. Unlike a subscription localization there is **no `state`**: `InAppPurchaseLocalizationV2` has none, because the state lives on the version. Import accepts a bare ID — the parent is resolved in two hops, localization → version → purchase — or the composite `<in_app_purchase_id>/<localization_id>`. **Delete tolerates Apple's 409 `Cannot delete the last localization`**: a version must keep at least one, so destroying the only localization warns and drops state rather than erroring — erroring failed the whole destroy and left the purchase behind, and the record goes away with the purchase anyway. `ImportState` sets `review_note` itself instead of going through `applyPurchaseAttributes`, which skips it so that a null in configuration survives a `Read`; an import has no configuration to contradict, and leaving it null dropped a value Apple holds.
- **`apple_in_app_purchase_price_schedule`**: a purchase has exactly **one** schedule, not a price record per territory the way a subscription does. `POST /v1/inAppPurchasePriceSchedules` replaces it wholesale, and there is no `PATCH` and no `DELETE` — so `Update` posts again (a genuine in-place update, not `RequiresReplace`) and `Delete` warns and drops state only. Both this and `apple_in_app_purchase_availability` need a `ModifyPlan` that marks `id` unknown when anything else changes: Terraform proposes the prior value for a computed attribute, Apple issues a new record ID on every `POST`, and without it every update would be rejected as an inconsistent result. The create request is the only one in the provider using JSON:API's `included` member: the prices do not exist yet, so each is sent inline under a literal placeholder ID (`${price0}`) that the `manualPrices` relationship references, and Apple substitutes real IDs on commit. `baseTerritory` is required — Apple added that after the endpoint shipped. `prices` is a **set**, because Apple returns them unordered, and `Read` matches by `price_point_id` and **keeps the configured dates** for a price already in state: Apple derives an `endDate` for any price a later one supersedes, and adopting it would be a permanent diff. Only `manualPrices` are read; the prices Apple equalizes from the base territory are computed and never the provider's. Import ID is the purchase ID.
- **`apple_in_app_purchase_availability`**: same singular, `POST`-replaces, no-`DELETE` shape as the price schedule, and the same consequences — `Update` posts again, `Delete` warns and drops state only. The record itself carries only `availableInNewTerritories`; the territory list is a separate paginated collection under `/v1/inAppPurchaseAvailabilities/{id}/availableTerritories`, so `Read` takes two requests. `available_in_new_territories` defaults to `true`, matching App Store Connect's own default — but it only covers storefronts Apple opens *later*, so it is no substitute for naming the current ones; see `apple_subscription_availability` above and `apple_territories`. Import ID is the purchase ID.
- **`apple_app_settings`**: the app record's own `PATCH`, and the only place
  `contentRightsDeclaration` lives — the answer App Store Connect blocks a
  submission on with "You must set up Content Rights Information in App
  Information". `name`, `bundle_id` and `sku` are computed: the first two are
  not updatable and the third would repoint a live app at a different App ID,
  which is not something an attribute should be able to do by accident, even
  though Apple's update request accepts it. `Read` deliberately does not refresh
  the optional URL attributes — Apple omits a URL it holds no value for, and
  writing null over a configured value produces a diff on the next plan.
- **`apple_app_info`**: categories, and nothing else — Apple's `AppInfo` update
  request has **no attributes at all**, only the six category relationships. An
  unset category is sent as an **explicit null** rather than omitted, which is
  why `AppInfoUpdateRelationships` uses `NullableResourceIdentifier` instead of
  the ordinary one: omitting a member leaves the existing category in place, so
  a category removed from the configuration would never actually be removed.
  `ResourceIdentifier` cannot express this — it marshals an unset value as
  `{"type":"","id":""}`, which Apple rejects.
- **`apple_app_info_localization`**: the app's name and subtitle, 30 characters
  each. Looked up **by locale on the editable app info**, not by the stored ID,
  for the reason given above. `Delete` tolerates Apple's refusal to remove the
  primary locale's record the way `apple_in_app_purchase_localization` does.
  Note that `privacy_policy_url` is a link on the product page and is **not** the
  App Privacy questionnaire, which has no API.
- **`apple_app_age_rating_declaration`**: the content questionnaire, ~30
  attributes, adopted rather than created. Every answer is `Optional+Computed`
  so an answer given in App Store Connect is adopted rather than fought over,
  with two exceptions that are `Optional` alone: `kids_age_band`, because an app
  outside the Kids Category has no band and a null is the same as unset, and
  `developer_age_rating_info_url`. **An omitted answer is not `NONE`** — Apple
  leaves it exactly as it was, which for a new app means unanswered, and an
  unanswered questionnaire blocks the submission. Apple revised this
  questionnaire and the provider models both forms: `INFREQUENT`/`FREQUENT`
  alongside `INFREQUENT_OR_MILD`/`FREQUENT_OR_INTENSE`, and
  `age_rating_override_v2` (where `SEVENTEEN_PLUS` became `EIGHTEEN_PLUS`)
  alongside `age_rating_override`. The booleans `age_assurance`, `loot_box`,
  `messaging_and_chat`, `parental_controls`, `social_media`,
  `social_media_age_restricted`, `user_generated_content`, `advertising`,
  `health_or_wellness_topics` and `guns_or_other_weapons` exist only on the
  newer form. A metadata file written against the older questionnaire — the
  attribute set EAS still emits — is missing all of them.
- **`apple_app_store_version`**: the one resource here with a real create and a
  real delete. `platform` is `RequiresReplace` and accepts `TV_OS` and
  `VISION_OS`, which an `apple_bundle_id` platform does not — the two lists are
  different and should not be shared. `version_string` is mutable in place.
  **An app holds only one editable version per platform at a time**, so `Create`
  maps Apple's 409 to a diagnostic naming the composite import ID. `Delete`
  warns rather than erroring when the version has reached review, since only an
  editable one can be removed. `Read` never refreshes `earliest_release_date`:
  Apple normalizes it to UTC whatever offset was sent, so adopting it rewrites a
  configured `2026-03-01T08:00:00-07:00` as `...T15:00:00Z` and shows a diff on
  every plan for a value that never changed.

  **The build is named by its number, not by Apple's ID.** `build_number`
  (`CFBundleVersion`) and `pre_release_version` (`CFBundleShortVersionString`,
  defaulting to `version_string`) are the two values a pipeline already exports,
  and the provider resolves them to the opaque build ID itself — the linkage
  endpoint takes an ID nobody has, so the lookup is unavoidable wherever it
  lives, and putting it here removes a builds data source, a `builds[0]` index
  expression and the index error an empty result would otherwise produce. Apple
  filters `/v1/builds` server-side on every coordinate (`filter[version]` is the
  build number, `filter[preReleaseVersion.version]` its train, plus
  `filter[preReleaseVersion.platform]`), so it is one request rather than the
  full-collection scan the other by-identifier helpers are forced into. Four
  things follow:
  - The build **cannot be set at creation** — `AppStoreVersionCreateRequest`
    carries only the `app` relationship — so it is always a second call, a
    `PATCH` to `/v1/appStoreVersions/{id}/relationships/build` answering 204
    with no body. `Create` therefore **writes state before reporting an
    attachment failure**: a created version left out of state is one the next
    apply cannot see, and Apple answers that with the 409 about the version
    already being prepared.
  - The linkage uses `NullableResourceIdentifier`, for the reason
    `AppInfoUpdateRelationships` does: detaching is an explicit `{"data":null}`,
    and an omitted member leaves the build in place, so a `build_number` removed
    from a configuration would never actually come off.
  - `Read` **does not refresh the build when `build_number` is null in state**,
    the same rule `apple_app_settings` follows for its optional URLs —
    otherwise a build attached by hand or by a separate pipeline lands in state
    and the next plan detaches it. It costs a configuration that does not use
    the attribute nothing: the extra request is skipped entirely.
    `pre_release_version` is never refreshed at all, because Apple reports the
    train as a relationship rather than a value on the build and reading it back
    would cost a request to say what `version_string` already says.
  - `build_id` is computed, so `ModifyPlan` marks it unknown when
    `build_number`, `pre_release_version` or `version_string` changes — the same
    inconsistent-result trap `apple_app_price_schedule` carries a `ModifyPlan`
    for. Every other change carries the stored ID forward untouched.

  The resource and the data source describe a version differently on purpose,
  the way `apple_bundle_id_capability` does: `appStoreVersionResourceModel`
  embeds `appStoreVersionModel` and adds the three build attributes, because a
  listing cannot report them without one request per version and the two numbers
  that name a build are inputs rather than anything Apple reports on a version.

  **Do not add polling on `processingState`.** A build is `PROCESSING` for five
  to thirty minutes after upload and Apple refuses to attach one until it is
  `VALID`; the resource reports the state it found rather than blocking an apply
  on it. For the same reason `GetBuilds` takes no processing-state filter —
  filtering a processing build away turns a wait into a "no such build".

  **Submission is deliberately not modelled.** The modern flow exists
  (`POST /v1/reviewSubmissions` → `POST /v1/reviewSubmissionItems` → `PATCH`
  with `submitted: true`; the older `appStoreVersionSubmissions` is gone), but
  it is an event rather than a state, and a resource for it would cancel a live
  review on destroy and resubmit on apply — the same irreversibility class as
  `apple_app_price_schedule` and `apple_app_availability`, which are guarded
  behind `APPLE_TEST_ALLOW_APP_PRICING` for it. Export compliance stays manual
  too: `usesNonExemptEncryption` lives on the **build**, not the version, and a
  build without it parks the version in `WAITING_FOR_EXPORT_COMPLIANCE` however
  complete the metadata is. It is normally answered by
  `ITSAppUsesNonExemptEncryption` in `Info.plist` at build time.
- **`apple_app_store_version_localization`**: **`keywords` is a Terraform list
  and a single comma-separated string at Apple**, capped at 100 characters
  including the commas. The provider joins with no space, because a space is a
  character and every one comes out of the hundred. The cap applies to the
  joined string, so no per-element validator can express it — `ValidateConfig`
  checks the joined length and reports the real number at plan time. The model
  field is `types.List`, **not `[]types.String`**: the whole list is unknown
  when it comes from a variable or a `for_each`, and a Go slice cannot represent
  that — decoding failed with "Received unknown value, however the target type
  cannot handle unknown values". The same applies to the `territories` and
  `platforms` filter attributes on the data sources. `make validate-examples` is
  what caught it; unit tests and `terraform fmt` did not.
- **`apple_app_store_review_detail`**: Apple attaches one to some versions and
  not others, so `write` reads first and patches when a record exists rather
  than posting blindly — a plain `POST` would 409 on exactly the versions that
  already had one. `Read` never refreshes `demo_account_password`: Apple returns
  it masked on some accounts and omitted on others, and adopting either
  overwrites the real value with something that is not it. It is `Sensitive`,
  but it is still in state in plain text and the schema says so.
- **`apple_app_price_schedule`**: the same `POST`-replaces, no-`PATCH`,
  no-`DELETE` shape as `apple_in_app_purchase_price_schedule`, including the
  `included`-member create with `${price0}` placeholders and the `ModifyPlan`
  that marks `id` unknown. It is a different Apple resource with its own price
  point catalogue — `/v1/apps/{id}/appPricePoints` — and an app's price points
  are not interchangeable with a subscription's or a purchase's. **Apple retired
  price tiers**; a free app is a price point whose `customerPrice` is `0`, not
  the absence of a schedule. Unlike the in-app purchase inline create, an
  `appPrices` object names only the price point: there is no second relationship
  repeating the parent.
- **`apple_app_availability`**: `/v2/appAvailabilities`, and a richer record
  than the in-app purchase and subscription availabilities. Where those carry a
  flat list of territory codes, this carries a `territoryAvailability` per
  storefront with its own release date and pre-order flag, sent inline under
  `${territory0}` placeholders. `write` sets `available: true` explicitly on
  every entry — a record sent without it is a storefront with no answer, not an
  omission Apple fills in. `Read` drops the storefronts Apple reports as
  unavailable and keeps the configured dates for one already in state, for the
  same reason the price schedule does: Apple fills in a release date for a
  storefront that has already shipped, and adopting it is a permanent diff.
  Additions are sorted before being appended, so Go's map iteration order does
  not churn the plan.
- **`apple_beta_group`**: the only TestFlight resource with a real create and a
  real delete. `is_internal_group` and `has_access_to_all_builds` are
  `Optional+Computed+RequiresReplace` with `UseStateForUnknown` — absent from
  Apple's update request, so neither can change in place, and neither carries a
  Terraform `Default`: a default would make an imported internal group plan a
  replacement the moment the configuration stayed silent about it. The remaining
  booleans are `Optional+Computed` and go through `adoptBool`/`adoptInt64` in
  `applyBetaGroup` rather than being assigned outright, because an attribute
  Apple omits from a response must neither blank a value the plan holds (an
  inconsistent result) nor stay unknown. Import accepts a bare Apple ID —
  `GET /v1/betaGroups/{id}` accepts `include=app`, which
  `apple_subscription_group`'s does not — or `<app_id>/<group_name>`; names are
  unique within an app, which is what makes the second form work and what
  `Create` maps Apple's duplicate error onto.
- **`apple_beta_tester`**: one tester in one group, and the only resource here
  whose Apple record is **not what it manages**. A beta tester is one record per
  email address for the whole account, shared by every group and app it belongs
  to, so `id` is that record's — two resources naming the same address carry the
  same `id` — and what each resource owns is the linkage. Four things follow:
  - `Create` **asks the account first**. A tester who already exists cannot be
    POSTed again, so the existing record is linked with
    `POST /v1/betaGroups/{id}/relationships/betaTesters` instead; a tester who is
    already in the group is an error naming the import ID, because adopting a
    membership silently would let a later destroy remove somebody a
    configuration never added. A `DUPLICATE` from the create is retried as that
    same link: two resources putting one person in two groups have nothing
    connecting them, so Terraform applies them at once and one loses the race —
    handling it here saves every such configuration a `depends_on`.
  - `Read` asks the **group's** collection (`GET /v1/betaGroups/{id}/betaTesters`),
    never the account-wide one. The question is membership: a tester who exists
    but has been taken out of the group has to read as gone, and a collection
    already scoped to the group cannot answer otherwise. **No `filter[email]` is
    sent here**, unlike on the account-wide `/v1/betaTesters`: Apple refuses it
    on the relationship endpoint with a 400 — "The parameter 'filter[email]' can
    not be used with this request" — which failed every create and read. The
    whole collection is walked and the match made in memory, case-insensitively.
  - Every attribute is `RequiresReplace` and `Update` exists only to report that
    it was reached, the way `apple_profile`'s does — Apple publishes no
    `PATCH /v1/betaTesters`. `first_name` and `last_name` are therefore sent only
    when the provider creates the record, are never refreshed, and `Create`
    **warns** when an adopted tester holds a different name rather than writing
    one Apple did not apply.
  - `Delete` removes the linkage, **not** `DELETE /v1/betaTesters/{id}`, which
    would remove the person from every app in the account. Import is
    `<group_id>/<email>` with no bare-ID form: an Apple tester ID names the
    person, not the membership, and the same ID belongs to every group they are
    in — the second resource in the provider with no bare form, for a different
    reason than `apple_beta_build_localization`.
- **`apple_beta_app_localization`**: the app's TestFlight page text, one record
  per locale, surviving every build. `Create` **reads first and patches when a
  record exists**, the way `apple_app_store_review_detail` does: Apple writes one
  for the app's primary locale itself, so a plain `POST` would 409 on exactly the
  locale a configuration starts with. `locale` is `RequiresReplace` — it
  identifies the record and is absent from the update request. `Delete` tolerates
  Apple's refusal to remove the last one. Note that `privacy_policy_url` here is
  the **TestFlight** link, not the App Store one on
  `apple_app_info_localization`, and neither is the App Privacy questionnaire.
- **`apple_beta_build_localization`**: the per-build "What to Test" note, and the
  one resource in the provider with **no bare-ID import form**. It hangs off a
  build, so it hits the same opaque-ID problem `apple_app_store_version` solved
  and solves it the same way: `build_number` + `pre_release_version` (+ optional
  `platform`) resolved through `GetBuildByNumber`, which is one request because
  `/v1/builds` filters server-side. `pre_release_version` is **required** here,
  unlike on `apple_app_store_version`, because there is no `version_string` to
  default it to. `Read` never refreshes the build coordinates — they are inputs,
  and Apple reports the train as a relationship rather than a value. Import is
  `<app_id>/<pre_release_version>/<build_number>/<locale>`, or the same with
  `<platform>` second. Like the app localization, `Create` adopts rather than
  posting blindly: Apple writes a note for the primary locale when a build
  finishes processing.
- **`apple_beta_app_review_detail`**: **adopted, never created.** Apple publishes
  no `POST /v1/betaAppReviewDetails` — the record comes into existence with the
  app — so `Create` is a `PATCH` and `Delete` warns and drops state, the shape
  `apple_app_settings` and `apple_app_info` are in. Do not add a create path.
  It is a different Apple record from `apple_app_store_review_detail` and the two
  do not substitute for each other: this one is per app and gates external
  TestFlight distribution, that one is per version and gates the App Store
  listing. `Read` never refreshes `demo_account_password`, for the reason the App
  Store one does not: Apple returns it masked on some accounts and omitted on
  others. It is `Sensitive` and still in state in plain text, and the schema says
  so. Import ID is the app ID.
- **`apple_user`**: adopted on create like `apple_app_settings`, and the only adopting resource in the provider with a **real delete** — `DELETE /v1/users/{id}` takes the person off the team and revokes their access to every app, which the API cannot undo. `Create` is a lookup that must succeed plus the same `PATCH` `Update` sends; an address that is not already a member is an error naming `apple_user_invitation`, because Apple publishes no way to add one. `username` is `RequiresReplace` and belongs to the person's Apple Account rather than to the team, which is also why `first_name` and `last_name` are **computed**: Apple's update request carries neither. Roles are a `types.Set` sorted before being sent, so two configurations naming the same roles produce the same body. `Read` looks the member up by Apple's ID rather than by username — a person who left and rejoined is not the same record — and refreshes `visible_apps` only when state holds one; see the `user` package notes above for why. `ACCOUNT_HOLDER` is refused in `ValidateConfig`, and Apple's refusal of a write naming the account holder is mapped to a diagnostic that says to transfer the membership rather than retry.
- **`apple_user_invitation`**: the only way to add a member, and a record Apple **destroys on acceptance**. Every configurable attribute is `RequiresReplace` (there is no `PATCH /v1/userInvitations`) and `Update` exists only to report that it was reached, the shape `apple_profile` and `apple_beta_tester` are in. `first_name` and `last_name` are **required here and nowhere else** — Apple rejects the create without them — which is what makes them computed on `apple_user`. The 404-means-three-things problem is handled in `Read` (see above) and again in `Delete`: an invitation already accepted cannot be cancelled, so `Delete` warns, drops state and names `apple_user` rather than deleting the member, which would turn removing a stale invitation from a configuration into removing a colleague. An invitation lapses after 72 hours, reported through the computed `expiration_date`.
- **`apple_profile`**: every configurable attribute is `RequiresReplace`, `name` included — Apple rejects `PATCH /v1/profiles` with 403 `does not allow 'UPDATE'`, so a rename is a reissue and `Update` exists only to report that it was reached. Profile content/UUID/state/dates are Apple-computed. `CreateProfile` retries a 5xx up to three times: `POST /v1/profiles` intermittently answers 500 for a well-formed request that succeeds on the next attempt. It looks the name up before each retry and adopts an already-issued profile, so a 500 that arrives after Apple committed the write does not duplicate it.

## Local testing

The provider serves `registry.terraform.io/ahmedosman00/apple` (set in `main.go`; lower case because Terraform normalizes source addresses, while the Registry displays the namespace as `AhmedOsman00`). `terraform init` in `examples/` therefore downloads the last released version — to exercise the working tree instead, wire a locally built binary up through a `dev_overrides` block in `~/.terraformrc`, or a filesystem mirror if `init` needs to keep working. Run with `-debug` to attach a debugger.

The examples pin `version = "~> 0.1"`, and `scripts/validate-examples.sh` builds the working tree into a mirror at version `0.1.0` to satisfy that constraint — bump `PROVIDER_VERSION` there if the pin ever moves past it.

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`: GoReleaser cross-compiles the provider (only `main.go`; the Registry matches every entry in `SHA256SUMS` against expected artifact names, so any extra binary built here would fail ingestion), signs the checksums with the GPG key in the `GPG_PRIVATE_KEY` / `PASSPHRASE` secrets, and attaches `terraform-registry-manifest.json`. `project_name` is pinned in `.goreleaser.yml` because the Registry requires `terraform-provider-apple_<version>_<os>_<arch>.zip`, and GoReleaser would otherwise take the name from the checkout directory. The Registry ingests new tags automatically once the repository is connected and the public GPG key is uploaded; update `CHANGELOG.md` before tagging.

## Tests and CI

Two tiers:

- **Credential-free** (`internal/apple/pagination_test.go`, `internal/apple/subscriptions_test.go`, `internal/apple/subscriptionPromotionalOffers_test.go`, `internal/apple/subscriptionIntroductoryOffers_test.go`, `internal/apple/winBackOffers_test.go`, `internal/provider/subscription/promotional_offer_test.go`, `internal/apple/inAppPurchases_test.go`, `internal/apple/builds_test.go`, `internal/apple/beta_test.go`, `internal/provider/bundle/capability_models_test.go`, `internal/provider/certificate/renewal_test.go`, `internal/provider/device/models_test.go`, `internal/provider/app/metadata_filters_test.go`, `internal/provider/app/schema_test.go`, `internal/provider/beta/schema_test.go`, `internal/apple/users_test.go`, `internal/provider/user/filters_test.go`, `internal/provider/user/schema_test.go`, `internal/provider/subscription/introductory_offer_test.go`, `internal/provider/subscription/win_back_offer_models_test.go`, `internal/provider/subscription/win_back_offer_schema_test.go`): `httptest`-backed client tests and pure-function tests. `apple.Client` has all-exported fields, so pointing one at a test server needs no production seam — `&apple.Client{HostURL: srv.URL, HTTPClient: srv.Client(), Token: "test"}`.
- **Acceptance** (`internal/provider/*_test.go`, package `provider`): `terraform-plugin-testing` against the real API. `testAccPreCheck` skips when credentials are absent.

All thirty-seven resources now have acceptance coverage. `internal/provider/app/schema_test.go`, `internal/provider/beta/schema_test.go` and `internal/provider/user/schema_test.go` are a second, cheaper net under the app listing, TestFlight and team membership resources: the framework validates a schema only when the provider server starts, so a Required+Computed attribute or a malformed nested block would otherwise surface only in the acceptance tier — which needs credentials and a real app and so never runs in CI. It runs every schema through `ValidateImplementation` and pins the resource type names, since renaming one is breaking. Checks go through `testAccAPIClient()` (provider_test.go), which builds a client from the same environment variables the provider reads: a `CheckDestroy` that only inspects Terraform state passes even when the resource is still live in the portal, so every existence and destroy check asks Apple. `testAccCheckDestroyAll` composes the checks for configurations that create several kinds of resource; the bundle ID checks predate this and still assert nothing.

Two things constrain what the acceptance tier is allowed to do:

- **Devices are permanent.** Apple has no device delete, so `device_resource_test.go` registers two fake UDIDs that stay in the team forever and consume device slots. Nothing new should register devices — which is why the profile tests use `IOS_APP_STORE` (needs no devices) rather than a development or ad hoc type. The UDIDs are hardcoded and `Delete` only disables, so a second run of those tests hits Apple's 409 and fails on `Device Already Exists`: they pass once per team, ever. CI skips them for that reason; run them by hand when you have accepted the slots.
- **App Store Connect tests need an app that already exists.** Apple's API cannot create an app record, so `TestAccSubscription*`, `TestAccInAppPurchase*` and `TestAccApps*` skip unless `APPLE_TEST_APP_ID` names one — `testAccPreCheckSubscription` enforces it, and `testAccPreCheckInAppPurchase` is a thin wrapper over it, and `.github/workflows/acceptance.yml` warns rather than passing silently when the secret is absent. Product identifiers are **randomised per run** here, deliberately breaking the fixed-identifier convention the rest of the suite follows: Apple reserves every product ID it has ever seen, so a fixed one would pass exactly once per account and fail forever after, the way the device tests do. In-app purchase product identifiers follow the same rule and the same helper shape (`testAccInAppPurchaseProductID`). **Product *names* are randomised too**, through `testAccProductName`: Apple enforces name uniqueness across an app's live products — "This name is already being used by another in-app purchase associated with this app", subscriptions included — so a fixed name collides with whatever a failed test left behind and with the test that ran a second earlier. Unlike an identifier a name is released when the product is deleted, so this is only about overlap, not permanent reservation. `TestAccInAppPurchaseResource_completesMetadata` additionally leaves a price schedule and an availability behind on a purchase it then deletes — both are undeletable in their own right, but deleting the purchase takes them with it. Randomising still consumes a handful of identifiers per run from an unlimited namespace, which is harmless; a fixed one would consume the only usable value.
- **Win-back offers need an approved subscription.** Apple creates one only on a subscription App Review has approved, which the suite cannot produce, so `TestAccWinBackOfferResource_basic` skips unless `APPLE_TEST_APPROVED_SUBSCRIPTION_ID` names one. Its `offer_id` is randomised (`testAccWinBackOfferID`) because Apple treats one as used once it has been, and its start date is thirty days out so the offer is never live while the test changes it.
- **The app listing tests edit an app rather than creating products.** Point
  `APPLE_TEST_APP_ID` at a scratch app: `TestAccAppInfoResource_categories`
  rewrites its categories, `TestAccAgeRatingDeclarationResource_basic` its age
  rating answers, and `TestAccAppInfoLocalizationResource_basic` its localized
  name and subtitle. Most of those adopt records Apple created, so there is
  nothing to delete and `testAccCheckAppStillExists` is the only honest
  `CheckDestroy` — the values stay on the app afterwards.
  `TestAccAppStoreVersionResource_basic` and the two tests that hang off a
  version do create a real version, deletable only while it is still being
  prepared, so an interrupted run leaves one to remove in App Store Connect; the
  version strings are randomised in the `99.x.y` range for that reason.
- **Two app listing tests are guarded past credentials.**
  `TestAccAppPriceScheduleResource_basic` and
  `TestAccAppAvailabilityResource_basic` change what an app costs and where it
  sells, and Apple publishes no `DELETE` for either record, so they cannot put
  back what was there. They skip unless `APPLE_TEST_ALLOW_APP_PRICING` is set —
  credentials plus a test app are deliberately not enough. Never point them at a
  live app.
- **The TestFlight tester test emails a real person.** Creating a membership
  makes Apple send a TestFlight invitation, so `TestAccBetaTesterResource_basic`
  skips unless `APPLE_TEST_BETA_TESTER_EMAIL` names an address the runner is
  entitled to invite — credentials and a test app are deliberately not enough,
  the same guard the app pricing tests are behind. Destroy removes the
  membership, so the tester record stays in the account afterwards: the provider
  never deletes one.
- **The TestFlight tests never open a public link.** `beta_resource_test.go`
  creates real beta groups and deletes them, which is neutral — a group with no
  testers and no public link invites nobody. Enabling `public_link_enabled`
  would issue a URL anyone can join the beta through, which is not something a
  test should put into the world, so the one configuration that would be
  rejected for it is covered by a `PlanOnly` `ExpectError` instead. Group names
  are randomised, because Apple enforces them unique within an app.
  `testAccBetaLocale` is `de-DE` rather than `en-US` on purpose: Apple writes a
  localization for the app's primary locale itself and refuses to delete the
  last one, so a test on the primary locale would adopt a record it could not
  then remove. `TestAccBetaAppReviewDetailResource_basic` adopts a record that
  cannot be deleted at all, so its contact details stay on the app afterwards
  and `testAccCheckAppStillExists` is its `CheckDestroy`.
  `TestAccBetaBuildLocalizationResource_basic` needs an uploaded build and skips
  behind `testAccPreCheckBuild` like the build attachment test does.
- **The team membership tests are the only ones that can remove a person.** `TestAccUserInvitationResource_basic` emails a real address, so it skips unless `APPLE_TEST_USER_INVITE_EMAIL` names one the runner is entitled to invite — the same guard the TestFlight tester test is behind. `TestAccUserResource_basic` goes further: its destroy removes the member from the team for good, so it needs `APPLE_TEST_USER_EMAIL` **and** `APPLE_TEST_ALLOW_USER_REMOVAL`, because naming somebody is not on its own a statement that losing them is acceptable. Neither needs `APPLE_TEST_APP_ID` — they write to the team rather than to an app — and both need an API key with the **Admin** role, which the rest of the suite does not. The roles the tests grant stop at `APP_MANAGER`: a test that granted `ADMIN` would leave a window an interrupted run could not close. `TestAccUsersDataSource_*`, `TestAccUserResource_notOnTheTeam` and the two plan-only refusal tests write nothing and run on credentials alone.
- **Certificates consume a slot while they exist.** `certificate_resource_test.go` and both profile test files issue one and revoke it on destroy, so a completed run is neutral; an interrupted run leaves a certificate to revoke by hand. `IOS_DEVELOPMENT` is preferred over a distribution type where the profile type allows it.

Identifiers are fixed rather than randomised, matching the existing tests, so an interrupted run leaves a Bundle ID, Merchant ID or profile that must be deleted in the portal before the test passes again.

`ACCEPTANCE_TESTING.md` is the per-test reference: what every `TestAcc*` creates at Apple, where it appears in the portal, what it leaves behind, the cleanup list for an interrupted run, and a staged run order that defers the two irreversible device tests. It is hand-written and unverified — update the matching table when you add or rename an acceptance test.

One trap it records: Bundle ID identifiers cannot contain the underscore the platform constants carry (`BundleIdentifierValidator` is `^[a-zA-Z0-9.-]+\.[a-zA-Z0-9.-]+$`), and Apple's App ID *name* accepts only alphanumerics and spaces. `testAccBundleIDPlatformFixture` does both substitutions for `TestAccBundleIDResource_platforms`; interpolating a platform constant straight into either field fails.

`.github/workflows/test.yml` is the credential-free lane and runs four independent jobs: `build` (build + lint), `generate` (docs diff), `examples` (`make validate-examples`), and `unit` (`go test ./...`, where the acceptance tests skip themselves for want of `TF_ACC` and credentials). It triggers on every pull request and on pushes to `main` only — a push to a branch with an open PR would otherwise run it twice for the same commit — and cancels superseded runs.

Acceptance tests live in their own workflow, `.github/workflows/acceptance.yml`, and run **only on `workflow_dispatch`**: they create billable resources in a real Apple team, use fixed identifiers that collide if two runs overlap, and leave material to clean up by hand when interrupted. Its inputs are the Terraform version (one per run, not a matrix), a `-run` pattern, and an `include_device_tests` boolean that defaults to false — `TestAccDeviceResource*` registers two UDIDs permanently and passes once per team, ever. The job fails fast if the three `APPLE_APP_STORE_CONNECT_*` secrets are absent, because a suite that skips itself would otherwise report green on a run somebody triggered deliberately, and it sets `cancel-in-progress: false` so a queued run never kills one mid-apply.

All eleven filter packages have table-driven tests (`internal/provider/*/filters_test.go`) covering filtering, sorting, limiting, and the malformed-pattern errors; the app listing filters and helpers are in `internal/provider/app/metadata_filters_test.go` alongside them, which also covers the keyword join/split round trip and the two reconcile functions that keep configured dates against Apple's derived ones. Resource CRUD paths are still only reachable through the acceptance tier.

Two behaviours worth knowing when writing filter tests, because they differ per package: `bundle` and `certificate` match `name_pattern` as a **glob** (`filepath.Match`, whole-string), while `device`, `merchant`, `passtypeid`, `profile`, `subscription`, `inapppurchase`, `app`, and `user` match it as a **regex** (substring unless anchored). `merchant`'s display-name pattern is additionally case-insensitive, and every `user` pattern is.
