# Security Policy

## Reporting a vulnerability

**Do not open a public issue for a security problem.**

Use GitHub's private vulnerability reporting: go to the
[Security tab](https://github.com/MathGaps/terraform-provider-apple/security/advisories/new)
and choose **Report a vulnerability**. That opens a private thread only the
maintainer can see, and it becomes a published advisory once a fix ships.

This is MathGaps's fork (FORK.md). A problem in the provider's own code is
upstream's too: report it
[there](https://github.com/AhmedOsman00/terraform-provider-apple/security/advisories/new)
as well.

Please include the provider version, the Terraform version, the configuration
that triggers the problem with any credentials redacted, and what an attacker
gains. A proof of concept helps but is not required.

Expect an acknowledgement within a week. This is a single-maintainer project,
so there is no formal response SLA beyond that; you will get a status update
whether or not the report is accepted.

## Supported versions

The provider is pre-1.0. Only the latest released version is supported — fixes
ship as a new tag rather than as a patch to an older minor.

## What this provider handles

Two things are worth knowing before you assess a finding.

**The App Store Connect API key is a team-wide credential.** The `.p8` private
key, together with the issuer ID and key ID, lets its holder issue and revoke
certificates, register devices, and change App Store pricing for the entire
team. Apple lets you download the `.p8` exactly once. The provider reads it from
`APPLE_APP_STORE_CONNECT_PRIVATE_KEY` or the `private_key` argument, mints one
ES256 JWT from it in `NewClient`, and does not retain the key afterwards — the
token has a 20-minute life, which outlives any single Terraform command, so
there is no refresh path and nothing to re-sign with. All three credential
attributes are marked sensitive and never appear in plan output. Anything that
causes the key or the token to be logged, written to state, or sent anywhere
other than `api.appstoreconnect.apple.com` is a vulnerability; report it.

**The code-signing key never enters Terraform, by design.** The
[signing module](../examples/signing) takes a certificate signing request — a
public key plus a signature over it — and nothing else. A CSR, an issued
certificate and a provisioning profile are all public documents, so state and
outputs carry no secret. A change that would require Terraform to hold a private
key, or that would write one to state, is a design regression, not a feature.

`certificate_content` and `profile_content` are marked `Sensitive` in the schema
anyway, as a conservative default. They are not secrets, and the signing module
deliberately calls `nonsensitive()` on both.

## What is out of scope

- The Apple App Store Connect API itself. Report those to Apple.
- Terraform state confidentiality in general. State holds every attribute the
  provider reads; protecting the backend is the operator's job.
- Findings that require an attacker who already holds your `.p8` key, or write
  access to your Terraform configuration or state.
- Vulnerability-scanner output with no demonstrated path through this code. A
  CVE in a transitive dependency the provider does not reach is worth a normal
  issue, not a private report — `govulncheck` runs in CI and reports reachable
  ones.

## Release integrity

Releases are built by GoReleaser in
[`.github/workflows/release.yml`](workflows/release.yml) and the checksum file
is signed with the project's GPG key, which is published on the
[Terraform Registry](https://registry.terraform.io/providers/ahmedosman00/apple/latest).
`terraform init` verifies that signature for you. Binaries from anywhere other
than the Registry or this repository's GitHub Releases are not ours.
