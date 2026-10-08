# Terraform Provider for Apple

> **This is MathGaps's fork** of [AhmedOsman00/terraform-provider-apple](https://github.com/AhmedOsman00/terraform-provider-apple),
> released to the OpenTofu Registry as `mathgaps/apple` because upstream is on
> the Terraform Registry only. The provider is upstream's work; [FORK.md](FORK.md)
> lists the little that differs. Feature requests and bugs in the provider
> itself belong [upstream](https://github.com/AhmedOsman00/terraform-provider-apple/issues).

A Terraform provider for the [Apple App Store Connect API](https://developer.apple.com/documentation/appstoreconnectapi),
built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

It manages the parts of the Apple Developer portal that a build pipeline depends
on — App IDs and their capabilities, signing certificates, registered devices,
Merchant IDs, Pass Type IDs, and provisioning profiles — as configuration rather
than as clicks in a web UI.

It is a replacement for `fastlane match`: Terraform owns the portal, you keep the
signing key, and nothing Terraform stores is secret.

## Contents

| | |
|---|---|
| [`docs/`](docs/) | Generated reference for every resource and data source |
| [`docs/guides/getting-started.md`](docs/guides/getting-started.md) | Credentials, installation, first configuration, importing |
| [`docs/guides/code-signing.md`](docs/guides/code-signing.md) | The `fastlane match` replacement, end to end |
| [`examples/signing/`](examples/signing/) | A runnable module that manages a full signing setup |

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- An Apple Developer Program team, and an App Store Connect API key with the
  **App Manager** role
- [Go](https://golang.org/doc/install) >= 1.25 — only to build the provider from
  source

## Installation

The provider is published on the OpenTofu Registry as
[`mathgaps/apple`](https://search.opentofu.org/provider/mathgaps/apple/latest),
so declaring it is enough — `terraform init` downloads it:

```terraform
terraform {
  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}
```

To run against an unreleased build of the provider, use a filesystem mirror or a
`dev_overrides` block; [the getting started guide](docs/guides/getting-started.md#installing-the-provider)
covers both, with the trade-off between them.

## Authentication

The provider mints a short-lived ES256 JWT from three values you get when
creating an App Store Connect API key under **Users and Access → Integrations →
App Store Connect API**:

| Argument | What it is | Environment variable |
|---|---|---|
| `issuer_id` | Team issuer ID (UUID) | `APPLE_APP_STORE_CONNECT_ISSUER_ID` |
| `api_key` | Key ID, 10 characters | `APPLE_APP_STORE_CONNECT_API_KEY` |
| `private_key` | Contents of the `.p8` file | `APPLE_APP_STORE_CONNECT_PRIVATE_KEY` |

Environment variables are read first; an explicit value in the `provider` block
overrides them. All three are sensitive and never appear in plan output.

```bash
export APPLE_APP_STORE_CONNECT_ISSUER_ID="69a6de70-1234-47e3-e053-5b8c7c11a4d1"
export APPLE_APP_STORE_CONNECT_API_KEY="ABCD123456"
export APPLE_APP_STORE_CONNECT_PRIVATE_KEY="$(cat AuthKey_ABCD123456.p8)"
```

Apple lets you download the `.p8` file once, and it is a team-wide credential —
anyone holding it can issue and revoke certificates for your team. Keep it in a
secret store, not in version control.

## Example usage

```terraform
terraform {
  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}

provider "apple" {
  # Reads APPLE_APP_STORE_CONNECT_* from the environment.
}

resource "apple_bundle_id" "app" {
  identifier = "com.example.myapp"
  name       = "My Example App"
  platform   = "IOS"
}

resource "apple_bundle_id_capability" "push" {
  bundle_id       = apple_bundle_id.app.id
  capability_type = "PUSH_NOTIFICATIONS"
}

resource "apple_device" "tester" {
  name     = "QA iPhone"
  udid     = "00008030-000A4D8E0AB8802E"
  platform = "IOS"
}

resource "apple_profile" "development" {
  name         = "My Example App Development"
  profile_type = "IOS_APP_DEVELOPMENT"
  bundle_id    = apple_bundle_id.app.id
  certificates = [apple_certificate.development.id]
  devices      = [apple_device.tester.id]

  # Profiles snapshot entitlements when Apple generates them.
  depends_on = [apple_bundle_id_capability.push]
}
```

## Resources and data sources

Every resource has a matching plural data source that lists the collection with
in-memory filtering, `sort_by`, `sort_order`, and `limit`.

| Resource | Data source | Manages |
|---|---|---|
| `apple_bundle_id` | `apple_bundle_ids` | App IDs |
| `apple_bundle_id_capability` | `apple_bundle_id_capabilities` | Capabilities on an App ID |
| `apple_certificate` | `apple_certificates` | Signing certificates |
| `apple_device` | `apple_devices` | Registered devices |
| `apple_merchant_id` | `apple_merchant_ids` | Apple Pay Merchant IDs |
| `apple_pass_type_id` | `apple_pass_type_ids` | Apple Wallet Pass Type IDs |
| `apple_profile` | `apple_profiles` | Provisioning profiles |

Those are Developer Portal resources. The App Store Connect side hangs off an
app record instead, which Apple's API cannot create — `apple_apps` reads one,
and there is deliberately no `apple_app` resource.

| Resource | Data source | Manages |
|---|---|---|
| — | `apple_apps` | App records (read-only) |
| — | `apple_territories` | The App Store's storefronts (read-only) |
| — | `apple_app_categories` | The App Store's category catalogue (read-only) |
| `apple_app_settings` | — | Content rights, primary language, notification URLs |
| `apple_app_info` | — | App Store categories |
| `apple_app_info_localization` | — | App name, subtitle and privacy policy link, per language |
| `apple_app_age_rating_declaration` | — | The content questionnaire behind the age rating |
| `apple_app_store_version` | `apple_app_store_versions` | A release: version string, copyright, release type |
| `apple_app_store_version_localization` | — | Description, keywords and release notes, per language |
| `apple_app_store_review_detail` | — | What App Review is told: contact, demo account, notes |
| `apple_app_price_schedule` | `apple_app_price_points` | What the app costs |
| `apple_app_availability` | — | The storefronts the app sells in |
| `apple_subscription_group` | `apple_subscription_groups` | Auto-renewable subscription groups |
| `apple_subscription_group_localization` | — | Customer-facing subscription group name |
| `apple_subscription` | `apple_subscriptions` | Auto-renewable subscriptions |
| `apple_subscription_localization` | — | Customer-facing subscription name and description |
| `apple_subscription_price` | `apple_subscription_price_points` | Subscription prices, per territory |
| `apple_subscription_availability` | — | The territories a subscription sells in |
| `apple_in_app_purchase` | `apple_in_app_purchases` | One-time purchases: consumables, non-consumables, non-renewing subscriptions |
| `apple_in_app_purchase_localization` | — | Customer-facing purchase name and description |
| `apple_in_app_purchase_price_schedule` | `apple_in_app_purchase_price_points` | The price of a one-time purchase |
| `apple_in_app_purchase_availability` | — | The territories a one-time purchase sells in |

Behaviour worth knowing before you plan against a real team:

- **Destroying an `apple_certificate` revokes it at Apple**, and every build
  already signed with it stops verifying. Apple also caps how many distribution
  certificates a team may hold, so one state should own them.
- **Devices cannot be deleted through Apple's API.** Removing an `apple_device`
  disables it and drops it from state, with a warning.
- **`apple_profile.profile_type` selects the distribution method**, and
  `platform` is computed from it rather than set.
- **Do not import an `apple_certificate` you are still using.** `csr_content`
  forces replacement and Apple does not reliably return the original CSR, so the
  next apply reissues — and revokes the original. Read it through the
  `apple_certificates` data source instead.
- **App Privacy has no API.** App Store Connect blocks a submission until the
  data-collection questionnaire is answered, and Apple publishes no endpoint for
  it — there is no `appDataUsages` resource in the App Store Connect API. It has
  to be answered once on the website. It is declared per app rather than per
  version, so it does not recur with each release.
- **Listing metadata is split across two lifetimes.** The app's name, subtitle,
  categories and privacy policy link live on `apple_app_info*` and survive every
  release; the description, keywords, promotional text and release notes live on
  `apple_app_store_version*` and are replaced with the version. A flat metadata
  file of the kind EAS and fastlane use hides that split; `examples/app-listing`
  shows how a configuration keeps one map and fans it out to both.
- **Listing metadata can only be written while a version is being prepared.**
  Apple freezes an app info once it is in review or distributed, and publishes no
  way to create a fresh one, so the categories, the localized name and the age
  rating cannot change until the current review finishes.
- **`apple_app_settings`, `apple_app_info` and `apple_app_age_rating_declaration`
  adopt records Apple already created.** Apple makes them with the app and
  publishes no `POST` for them, so these resources patch rather than create, and
  `terraform destroy` drops them from state with a warning.
- **App Store Connect resources are incomplete until their metadata exists.** A
  new `apple_subscription` or `apple_in_app_purchase` reports `MISSING_METADATA`
  until its localization, price and — for a one-time purchase — availability are
  applied. Submitting for review stays a manual step.
- **A price schedule and an availability cannot be destroyed.** Apple publishes
  no `DELETE` for either, so `terraform destroy` drops them from state and warns;
  the values stay as last set. Narrow `available_territories` rather than
  destroying.
- **Apple never releases a product identifier.** Replacing an
  `apple_in_app_purchase.product_id`, or destroying the purchase, reserves that
  identifier forever — including for purchases deleted before review.

## Replacing fastlane match

[`examples/signing/`](examples/signing/) is a complete, runnable module that
manages everything `fastlane match` manages in the developer portal — App ID,
capabilities, devices, signing certificates, and one provisioning profile per
distribution method.

**You generate the private key; Terraform never sees it.** The module takes a
certificate signing request, which is public, and exports the issued certificate
and the generated profiles, which are also public. Nothing in the state or the
outputs is secret:

```bash
openssl genrsa -out distribution.key 2048
openssl req -new -key distribution.key -out distribution.csr -subj "/CN=My App distribution"

export TF_VAR_csr_contents='{"distribution":"'"$(cat distribution.csr)"'"}'
terraform -chdir=examples/signing apply

terraform -chdir=examples/signing output -json certificates
terraform -chdir=examples/signing output -json profiles
```

Installing the result is a `.p12` built from that certificate and your key, plus
some files dropped where Xcode reads them. Read [the code signing
guide](docs/guides/code-signing.md) for the exact commands — including the
PKCS#12 encoding trap that makes `/usr/bin/openssl` the only safe choice — and
for how to migrate off match by adopting its existing certificate instead of
reissuing, which would revoke it.

## Development

```shell
make            # fmt + lint + install + generate
make build      # go build -v ./...
make test       # unit tests; no credentials needed
make testacc    # acceptance tests against the real API
make generate   # regenerate docs/ from schemas, examples/, and templates/
make validate-examples  # terraform validate over every directory under examples/
```

Run a single test with `go test -v ./internal/apple/ -run TestGetAllPages`.

`go test ./...` is safe without credentials — acceptance tests skip themselves
when the App Store Connect environment variables are absent. Acceptance tests hit
the real API and create billable resources.

[`ACCEPTANCE_TESTING.md`](ACCEPTANCE_TESTING.md) lists every acceptance test,
what it creates in your Apple Developer account, and what it leaves behind. Read
it before pointing the suite at a team you care about: two of the tests register
devices, which Apple cannot delete.

### Documentation

`docs/` is generated by `tfplugindocs` from the `MarkdownDescription` strings on
each schema plus the matching example under `examples/`, so edit those rather
than the generated Markdown. Hand-written guides live in `templates/guides/` and
render into `docs/guides/`. CI fails if `make generate` produces a diff, so
regenerate and commit whenever a schema, example, or guide changes.

`make generate` only runs `terraform fmt` over `examples/`, which cannot catch
configuration the provider schema rejects. `make validate-examples` builds the
provider into a throwaway filesystem mirror and runs `terraform validate` in
every example directory; that is the check that catches an example written
against a schema the provider does not have.

## Contributing

Bug reports and pull requests are welcome. [`CONTRIBUTING.md`](.github/CONTRIBUTING.md)
covers the development setup and the four things about this codebase that are
easy to miss — registering new resources by hand, keeping the API client free of
Terraform types, paginating every list endpoint, and regenerating `docs/`.

Security problems go through [`SECURITY.md`](.github/SECURITY.md) rather than the
issue tracker.

## License

[MPL-2.0](LICENSE).
