---
page_title: "Getting Started"
subcategory: ""
description: |-
  Create App Store Connect API credentials, install the provider, and manage
  your first Bundle ID, certificate, device, and provisioning profile.
---

# Getting Started

This guide takes you from an empty directory to a Bundle ID, a signing
certificate, a registered device, and a provisioning profile — the four things
every iOS build needs — managed as Terraform configuration.

If what you actually want is to replace `fastlane match`, read
[Code Signing](./code-signing.md) instead. It builds on this guide and uses a
ready-made module rather than hand-written resources.

## Prerequisites

- Terraform >= 1.0.
- Membership in an Apple Developer Program team, with a role that can create API
  keys: **Account Holder** or **Admin**.
- Go >= 1.25, only if you want to build an unreleased provider from source —
  see [Installing the provider](#installing-the-provider).

## Creating App Store Connect API credentials

The provider authenticates with an App Store Connect API key, not an Apple ID
and password. Every request carries a short-lived ES256 JSON Web Token the
provider mints from three values.

1. Sign in to [App Store Connect](https://appstoreconnect.apple.com/) and go to
   **Users and Access** → **Integrations** → **App Store Connect API**, on the
   **Team Keys** tab.
2. Select **+** to generate a key. Give it the **App Manager** role. A Developer
   key is scoped to that person's own certificates and profiles, which is not
   enough to manage a team's signing material. Managing the team itself —
   `apple_user` and `apple_user_invitation` — needs an **Admin** key instead: an
   App Manager key can read `/v1/users` and not write to it.
3. Note the **Issuer ID** shown above the key list. It is a UUID, and it is the
   same for every key on the team.
4. Note the **Key ID** of the new key. It is 10 uppercase alphanumeric
   characters.
5. Download the `.p8` private key file. **Apple lets you download it once.** If
   you lose it, revoke the key and generate another.

Those three values map onto the provider's three arguments:

| Argument | What it is | Environment variable |
|---|---|---|
| `issuer_id` | Team issuer ID (UUID) | `APPLE_APP_STORE_CONNECT_ISSUER_ID` |
| `api_key` | Key ID (10 characters) | `APPLE_APP_STORE_CONNECT_API_KEY` |
| `private_key` | Contents of the `.p8` file, PEM included | `APPLE_APP_STORE_CONNECT_PRIVATE_KEY` |

Environment variables are read first and an explicit value in the `provider`
block overrides them, so you can keep credentials out of configuration entirely:

```bash
export APPLE_APP_STORE_CONNECT_ISSUER_ID="69a6de70-1234-47e3-e053-5b8c7c11a4d1"
export APPLE_APP_STORE_CONNECT_API_KEY="ABCD123456"
export APPLE_APP_STORE_CONNECT_PRIVATE_KEY="$(cat AuthKey_ABCD123456.p8)"
```

Note that `private_key` wants the **contents** of the file, not a path, PEM
armour included — the `.p8` Apple gives you is already the PKCS#8
`-----BEGIN PRIVATE KEY-----` form the provider expects. All three arguments are
marked sensitive, so they will not appear in plan output.

The provider validates all three before making a request, which turns a typo into
a plan-time error rather than a 401. The issuer ID check is lowercase-hex, matching
how App Store Connect displays it; the key ID check is exactly ten characters of
`A-Z0-9`.

~> **The `.p8` key is a team-wide credential.** Anyone holding it can issue and
revoke certificates for your team. Keep it in a secret store, not in version
control, and prefer the environment variables above over a `.tfvars` file that
is easy to commit by accident.

## Installing the provider

The provider is published on the OpenTofu Registry as
[`mathgaps/apple`](https://search.opentofu.org/provider/mathgaps/apple/latest).
Declare it and `terraform init` downloads it — there is nothing to build:

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

From a clone, `make install` builds the provider into `$GOPATH/bin`.

## Running an unreleased build

Only needed when you are changing the provider itself, or want a fix that is
merged but not tagged. There are two ways to make Terraform use a local binary,
and the difference matters more than it looks.

### Filesystem mirror — `init` works

A mirror is a directory laid out the way Terraform expects a provider registry
to be. It is the option to choose for anything beyond a scratch experiment,
because `terraform init` behaves normally and module installation works.

```bash
MIRROR=~/.terraform-mirror/registry.terraform.io/mathgaps/apple/0.1.0/$(go env GOOS)_$(go env GOARCH)
mkdir -p "$MIRROR"
go build -o "$MIRROR/terraform-provider-apple_v0.1.0" .
```

Then in `~/.terraformrc`:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/Users/you/.terraform-mirror"
    include = ["registry.terraform.io/mathgaps/*"]
  }
  direct {
    exclude = ["registry.terraform.io/mathgaps/*"]
  }
}
```

The version in the mirror path has to satisfy whatever your configuration pins,
and the mirror shadows the Registry for that address — remove the block when you
want released versions back.

`scripts/validate-examples.sh` in the repository does exactly this against a
throwaway mirror, if you want a worked example.

### Development override — `init` refuses to run

```hcl
provider_installation {
  dev_overrides {
    "mathgaps/apple" = "/Users/you/go/bin"
  }
  direct {}
}
```

An override makes Terraform use the binary at that path and ignore version
constraints, which is convenient while changing provider code. The cost is that
**`terraform init` fails outright** while an override is active; you skip `init`
and run `terraform plan` directly. That is fine for a single directory with no
modules and no backend, and painful for anything else.

## Your first configuration

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

# The App ID.
resource "apple_bundle_id" "app" {
  identifier = "com.example.myapp"
  name       = "My Example App"
  platform   = "IOS"
}

# An entitlement on that App ID.
resource "apple_bundle_id_capability" "push" {
  bundle_id       = apple_bundle_id.app.id
  capability_type = "PUSH_NOTIFICATIONS"
}

# A device you want development builds to run on.
resource "apple_device" "tester" {
  name     = "QA iPhone"
  udid     = "00008030-000A4D8E0AB8802E"
  platform = "IOS"
}
```

Run it:

```bash
terraform init      # skip this if you used dev_overrides
terraform plan
terraform apply
```

`apple_bundle_id.identifier` and `platform` are immutable at Apple, so changing
either replaces the resource. `name` updates in place.

## Adding a certificate and a profile

A certificate is issued from a Certificate Signing Request, so you need a
private key and a CSR before Apple has anything to sign. You can generate them
with `openssl`:

```bash
openssl genrsa -out development.key 2048
openssl req -new -key development.key -out development.csr \
  -subj "/CN=My Example App Development/O=Example Inc/C=US"
```

Or, better, let Terraform generate them with the `tls` provider so the key is
managed alongside everything else:

```terraform
resource "tls_private_key" "development" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_cert_request" "development" {
  private_key_pem = tls_private_key.development.private_key_pem

  subject {
    common_name  = "My Example App Development"
    organization = "Example Inc"
  }
}

resource "apple_certificate" "development" {
  certificate_type = "IOS_DEVELOPMENT"
  csr_content      = tls_cert_request.development.cert_request_pem
}

resource "apple_profile" "development" {
  name         = "My Example App Development"
  profile_type = "IOS_APP_DEVELOPMENT"
  bundle_id    = apple_bundle_id.app.id
  certificates = [apple_certificate.development.id]
  devices      = [apple_device.tester.id]

  # Profiles snapshot entitlements when Apple generates them, so the
  # capabilities have to exist first.
  depends_on = [apple_bundle_id_capability.push]
}
```

Three things about this configuration are worth internalising before you scale
it up.

**`profile_type` chooses the distribution method, and `platform` follows from
it.** An `IOS_APP_STORE` profile reports `platform = "IOS"` because Apple derives
it; you do not set `platform` on a profile. Development and Ad Hoc types require
`devices`; App Store and in-house types do not take them.

**Certificates are scarce and destroying one revokes it.** Apple caps how many
distribution certificates a team may hold — a small number, shared across every
developer and CI job. `terraform destroy` on an `apple_certificate` revokes it at
Apple, and every build already signed with it stops verifying. This is the single
biggest reason not to let each developer run their own copy of a configuration
that issues certificates.

**Devices are disabled rather than deleted.** Apple's API has no device delete,
so removing an `apple_device` issues a `PATCH` setting its status to `DISABLED`,
drops it from state, and emits a warning. The registration still counts against
your team's device limit until the annual membership renewal.

## Storing the state

If your configuration generates private keys with `tls_private_key`, those keys
are in the state file in plaintext. Give that configuration its own remote
backend with locking and versioning, and restrict read access to it as tightly as
you would restrict the signing keys themselves — because that is what it holds.
[Code Signing](./code-signing.md) works through what that means in practice.

## Importing what you already have

You almost certainly have identifiers, devices, and profiles in the developer
portal already. Every resource in this provider accepts either Apple's opaque ID
or the human-readable identifier you know it by, and works out which you gave it
from the string's shape:

| Resource | Accepted import IDs |
|---|---|
| `apple_bundle_id` | Apple ID, or bundle identifier (`com.example.myapp`) |
| `apple_bundle_id_capability` | `<bundle_id>/<capability_id>` |
| `apple_certificate` | Apple ID, serial number, or display name |
| `apple_device` | Apple ID, or UDID |
| `apple_merchant_id` | Apple ID, or identifier (`merchant.com.example.myapp`) |
| `apple_pass_type_id` | Apple ID, or identifier (`pass.com.example.loyalty`) |
| `apple_profile` | Apple ID, or profile name |

The App Store Connect resources are stricter, because Apple reports less about
them. Anything whose parent cannot be read back takes a composite ID:

| Resource | Accepted import IDs |
|---|---|
| `apple_app_settings` | App ID, or bundle identifier |
| `apple_app_info` | App ID |
| `apple_app_info_localization` | Apple ID, or `<app_id>/<locale>` |
| `apple_app_age_rating_declaration` | App ID |
| `apple_app_store_version` | Apple ID, or `<app_id>/<platform>/<version_string>` |
| `apple_app_store_version_localization` | Apple ID, or `<app_store_version_id>/<locale>` |
| `apple_app_store_review_detail` | `<app_store_version_id>` |
| `apple_app_price_schedule` | `<app_id>` |
| `apple_app_availability` | `<app_id>` |
| `apple_beta_group` | Apple ID, or `<app_id>/<group_name>` |
| `apple_beta_tester` | `<group_id>/<email>` |
| `apple_beta_app_localization` | Apple ID, or `<app_id>/<locale>` |
| `apple_beta_build_localization` | `<app_id>/<pre_release_version>/<build_number>/<locale>`, or the same with `<platform>` second |
| `apple_beta_app_review_detail` | `<app_id>` |
| `apple_subscription_group` | `<app_id>/<group_id>` |
| `apple_subscription_group_localization` | Apple ID, or `<group_id>/<localization_id>` |
| `apple_subscription` | Apple ID |
| `apple_subscription_localization` | Apple ID, or `<subscription_id>/<localization_id>` |
| `apple_subscription_price` | `<subscription_id>/<price_id>` |
| `apple_subscription_price_schedule` | `<subscription_id>` |
| `apple_subscription_availability` | `<subscription_id>` |
| `apple_in_app_purchase` | `<app_id>/<in_app_purchase_id>` |
| `apple_in_app_purchase_localization` | Apple ID, or `<in_app_purchase_id>/<localization_id>` |
| `apple_in_app_purchase_price_schedule` | `<in_app_purchase_id>` |
| `apple_in_app_purchase_availability` | `<in_app_purchase_id>` |
| `apple_user` | Apple ID, or the address the member signs in with |
| `apple_user_invitation` | Apple ID, or the address it was sent to |

```bash
terraform import apple_bundle_id.app com.example.myapp
terraform import apple_device.tester 00008030-000A4D8E0AB8802E
terraform import apple_profile.development "My Example App Development"
```

Import `apple_bundle_id_capability` with both halves — Apple's capability
response does not name its parent Bundle ID, so a bare capability ID imports with
`bundle_id` null and the next plan will force replacement:

```bash
terraform import apple_bundle_id_capability.push "ABCD123456/XYZW987654"
```

`apple_subscription_group` and `apple_in_app_purchase` need the app ID for the
same reason: Apple never reports which app they belong to, so a bare ID imports
with `app_id` null — and `app_id` forces replacement, which for an in-app
purchase would destroy it and reserve its product identifier forever.

The last two are the odd ones. A purchase has exactly one price schedule and one
availability record, and Apple publishes no collection of either, so the import
ID is the purchase itself:

```bash
terraform import apple_in_app_purchase.pro_unlock "6478123456/6739472901"
terraform import apple_in_app_purchase_price_schedule.pro_unlock 6739472901
```

Most of the app listing resources import by the **app ID** rather than by the ID
of the record they manage. That is not a shortcut: Apple issues a new app info
record — and a new age rating declaration, and new localization IDs with them —
for each version cycle, so the ID of the one being edited today is not stable
enough to name a resource by. The app is.

An `apple_app_store_version` is the exception, because a version really is a
durable record. Its composite form needs all three parts, since the same version
string exists once per platform:

```bash
terraform import apple_app_settings.app 6478123456
terraform import apple_app_info.app 6478123456
terraform import apple_app_store_version.v1 "6478123456/IOS/1.0"
terraform import 'apple_app_info_localization.this["en-US"]' "6478123456/en-US"
```

Importing the version already being prepared is the usual first move: an app
holds only one editable version per platform at a time, so `apple_app_store_version`
cannot create a second, and Apple answers the attempt with a 409. Find it with
the `apple_app_store_versions` data source filtered to
`app_version_state = "PREPARE_FOR_SUBMISSION"`.

The TestFlight resources mostly follow the App Store ones. `apple_beta_group`
and `apple_beta_app_localization` both accept a bare Apple ID, because Apple
reports the owning app on each; `apple_beta_app_review_detail` imports by app
ID, since an app has exactly one and there is no collection to find it in.

`apple_beta_build_localization` and `apple_beta_tester` are the two resources in
the provider with **no bare-ID form at all**, for different reasons. The build
note names its build by Apple's opaque build ID, and the build number and its
train cannot be recovered from that without two further requests — while whoever
is importing already has both. A tester's Apple ID names the *person* rather than
the membership, and the same ID belongs to every group that person is in, so it
cannot say which membership was meant:

```bash
terraform import apple_beta_group.qa "6478123456/QA"
terraform import apple_beta_app_review_detail.app 6478123456
terraform import 'apple_beta_build_localization.notes["en-US"]' "6478123456/1.4.0/42/en-US"
terraform import 'apple_beta_tester.qa["ada@example.com"]' "4f1a9c30-7b52-4e18-9d66-2c8a3f5e1b04/ada@example.com"
```

The two team membership resources take the address as readily as Apple's ID,
because nobody has the ID: `/v1/users` is a collection with no single-record
read, so the provider is scanning it either way.

```bash
terraform import apple_user.developer "ada@example.com"
terraform import apple_user_invitation.new_hire "grace@example.com"
```

Only a **pending** invitation can be imported. Apple destroys the record the
moment somebody accepts and issues a team member in its place, so an accepted
invitation has nothing left to import — bring the member in as an `apple_user`
instead.

!> **Do not import `apple_certificate` if you are still using the certificate.**
`csr_content` forces replacement and Apple does not reliably return the CSR a
certificate was issued from, so an imported certificate is reissued on the next
apply — which revokes the original. Read it through the `apple_certificates` data
source instead, or see the adoption approach in
[Code Signing](./code-signing.md).

## Reading instead of managing

Every resource has a matching plural data source (`apple_bundle_ids`,
`apple_certificates`, `apple_devices`, `apple_merchant_ids`,
`apple_pass_type_ids`, `apple_profiles`, `apple_bundle_id_capabilities`,
`apple_apps`, `apple_subscription_groups`, `apple_subscriptions`,
`apple_in_app_purchases`, `apple_users`) for referring to things another state
owns.
Two data sources have no resource behind them at all: `apple_apps`, because
Apple's API cannot create an app record, and `apple_territories`, because
territories are the App Store's rather than the account's.

Two of them are catalogues rather than listings: `apple_subscription_price_points`
and `apple_in_app_purchase_price_points` read Apple's permitted prices, which is
where the `price_point_id` a price references comes from. Always pass
`territories` — the unfiltered catalogue covers every storefront Apple sells in.

`apple_territories` is neither: it reads the storefronts themselves, and takes no
argument at all. It is how an availability record says "everywhere". Both
`apple_subscription_availability` and `apple_in_app_purchase_availability` require
`available_territories`, and Apple's default of every territory applies only to a
product whose availability has never been set — so once Terraform owns the record,
worldwide has to be spelled out:

```terraform
data "apple_territories" "all" {}

resource "apple_subscription_availability" "worldwide" {
  subscription_id              = apple_subscription.pro_monthly.id
  available_territories        = data.apple_territories.all.ids
  available_in_new_territories = true
}
```

`available_in_new_territories` covers only storefronts Apple opens later, so it is
not a substitute for the list.

```terraform
data "apple_certificates" "distribution" {
  certificate_type = "IOS_DISTRIBUTION"
  sort_by          = "expiration_date"
  sort_order       = "desc"
  limit            = 1
}
```

Apple's API does no server-side filtering for these collections, so the provider
fetches every page and filters in memory. Filter arguments, `sort_by`,
`sort_order` and `limit` all apply after the fetch, and `total_count` alongside
`filtered_count` tells you how much was discarded.

One inconsistency to know about: `name_pattern` is a shell-style glob matched
against the whole string on `apple_bundle_ids` and `apple_certificates`, and a
regular expression matched as a substring on `apple_devices`,
`apple_merchant_ids`, `apple_pass_type_ids` and `apple_profiles`.

## Where to go next

- [Code Signing](./code-signing.md) — the `fastlane match` replacement, and how to
  distribute signing material to people and CI.
- The per-resource pages in the sidebar, each with a runnable example.
