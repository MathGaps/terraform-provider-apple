# Examples

These directories serve two purposes: `tfplugindocs` embeds them into the
generated pages under `docs/`, and every one of them is checked by
`make validate-examples`, which runs `terraform validate` against a locally
built provider.

If you are looking for prose rather than configuration, start with
[`docs/guides/getting-started.md`](../docs/guides/getting-started.md) and
[`docs/guides/code-signing.md`](../docs/guides/code-signing.md).

## Layout

`tfplugindocs` looks for examples at fixed paths. A resource page embeds
`resources/<type>/resource.tf`, a data source page embeds
`data-sources/<type>/data-source.tf`, and the provider index page embeds
`provider/provider.tf`. Files anywhere else are ignored by the docs tool but
still validated.

| Path | Rendered into |
|---|---|
| `provider/provider.tf` | The provider index page |
| `resources/apple_*/resource.tf` | That resource's page |
| `data-sources/apple_*/data-source.tf` | That data source's page |
| `main.tf` | Nothing — a runnable tour of every resource together |
| `signing/` | Nothing — see below |

There is one directory per resource and data source the provider registers:
`apple_bundle_id`, `apple_bundle_id_capability`, `apple_certificate`,
`apple_device`, `apple_merchant_id`, `apple_pass_type_id`, and `apple_profile`,
plus their plural data source counterparts.

## `signing/` — the fastlane match replacement

[`signing/`](signing/) is not a documentation snippet. It is a complete, runnable
module that manages a full signing setup — App ID, capabilities, devices,
certificates, and one provisioning profile per distribution method. You generate
the signing key and give it a CSR; it exports the issued certificate and the
profiles, none of which are secret.

It has [its own README](signing/README.md), and
[`docs/guides/code-signing.md`](../docs/guides/code-signing.md) explains the
distribution model around it. Read one of them before pointing it at a real
team: applying it issues certificates, and Apple caps how many a team may hold.

## Running an example

Set credentials (see the [getting started
guide](../docs/guides/getting-started.md) for how to create them):

```bash
export APPLE_APP_STORE_CONNECT_ISSUER_ID="your-issuer-id"
export APPLE_APP_STORE_CONNECT_API_KEY="your-api-key-id"
export APPLE_APP_STORE_CONNECT_PRIVATE_KEY="$(cat AuthKey_XXXXXXXXXX.p8)"
```

The resource and data source directories carry no `terraform` block of their own,
because the docs pages read better without one. To run one directly, add the
source address:

```hcl
terraform {
  required_providers {
    apple = {
      source  = "mathgaps/apple"
      version = "~> 0.1"
    }
  }
}
```

Then `terraform init && terraform plan`. `init` downloads the provider from the
Terraform Registry; `make validate-examples` instead builds the working tree into
a throwaway filesystem mirror, which is what you want when checking an example
against unreleased schema changes.

~> These examples create real resources in your Apple Developer account, against
real quotas. Review the plan before applying.

## Placeholders you must replace

### Certificate Signing Requests

Certificate examples carry placeholder `csr_content`. A real CSR is generated
from a private key you keep:

```bash
openssl genrsa -out private.key 2048
openssl req -new -key private.key -out request.csr \
  -subj "/C=US/ST=CA/L=San Francisco/O=Your Company/CN=Your Name"
```

Apple ignores the CSR subject and issues under your team's name, so the subject
is cosmetic. The key is not: a certificate is only usable together with the key
its CSR was generated from. `signing/` generates both with the `tls` provider so
Terraform manages them together — see the [getting started
guide](../docs/guides/getting-started.md#adding-a-certificate-and-a-profile).

### Device UDIDs

Device examples carry placeholder UDIDs. Three shapes are in circulation and the
provider accepts all of them, leaving the final judgement to Apple:

| Device | Where to find it | Shape |
|---|---|---|
| iPhone / iPad / iPod touch / Apple Watch | Xcode → Window → Devices and Simulators, or Finder | 40 hex characters on iPhone X and earlier; `00008030-000A4D8E0AB8802E` on iPhone XS and later |
| Mac | `system_profiler SPHardwareDataType \| grep "Hardware UUID"` | UUID |
| Apple TV / Vision Pro | Settings → General → About → Identifier | UUID |

Apple Watches register under the `IOS` platform, not a platform of their own.

## Syntax that is easy to get wrong

**`settings` is a list of objects, not a block.** The schema is a
`ListNestedAttribute`, so it takes `=` and square brackets:

```hcl
resource "apple_bundle_id_capability" "icloud" {
  bundle_id       = apple_bundle_id.app.id
  capability_type = "ICLOUD"

  settings = [
    {
      key   = "ICLOUD_VERSION"
      value = "XCODE_5"
    },
  ]
}
```

**Profiles take `profile_type`, not `platform`.** Apple's create endpoint takes a
profile type that encodes both the platform and the distribution method;
`platform` is computed from it and cannot be set.

```hcl
resource "apple_profile" "development" {
  name         = "My App Development"
  profile_type = "IOS_APP_DEVELOPMENT" # not platform = "IOS"
  bundle_id    = apple_bundle_id.app.id
  certificates = [apple_certificate.development.id]
  devices      = [apple_device.tester.id]
}
```

Development and Ad Hoc types require `devices`; App Store and in-house types do
not take them.

## Behaviour these examples do not make obvious

- **Destroying an `apple_certificate` revokes it at Apple.** Builds already
  signed with it stop verifying. Apple also caps distribution certificates per
  team, so one state should own them.
- **Devices are disabled, not deleted.** Apple's API has no device delete, so
  removing an `apple_device` patches its status to `DISABLED`, drops it from
  state, and warns. The registration still counts against your device limit
  until the annual membership renewal.
- **Capabilities must exist before profiles are generated.** Profiles snapshot
  entitlements at generation time, so add a `depends_on` when both are in the
  same configuration.
- **Do not import an `apple_certificate` you are still using.** `csr_content`
  forces replacement and Apple does not reliably return the original CSR, so the
  next apply reissues and revokes the original. Read it through the
  `apple_certificates` data source instead.
- **Identifiers are immutable.** `identifier` and `platform` on a Bundle ID, a
  Merchant ID's or Pass Type ID's `identifier`, and a device's `udid` all force
  replacement. Display names and profile names update in place.

## Regenerating the docs

`make generate` runs `terraform fmt` over this directory and rebuilds `docs/`.
CI fails if that produces a diff, so run it after changing an example.
`terraform fmt` only checks syntax; run `make validate-examples` to catch
configuration the provider schema rejects.
