# MathGaps's fork

This is a fork of [AhmedOsman00/terraform-provider-apple](https://github.com/AhmedOsman00/terraform-provider-apple),
by AO Studio, under the Mozilla Public License 2.0 (`LICENSE`, unchanged). Every
resource, data source and line of provider logic here is upstream's work.

The fork exists for one reason: upstream is published on the Terraform Registry
only, and OpenTofu resolves `ahmedosman00/apple` against `registry.opentofu.org`,
where it is not listed. This fork releases the same code to the OpenTofu
Registry as `mathgaps/apple`.

## What differs from upstream

| Change | Where |
|---|---|
| The registry address a configuration names: `mathgaps/apple` | everything `scripts/mathgaps-registry.sh` rewrites: `main.go`, `examples/`, `templates/`, `docs/`, `scripts/validate-examples.sh`, `README.md` |
| The release job runs in the `release` environment, which holds the signing key and admits only `v*` tags | `.github/workflows/release.yml` |
| GoReleaser makes the release a draft and the workflow publishes it once every asset is attached, so the OpenTofu registry never scans half a release | `.goreleaser.yml`, `.github/workflows/release.yml` |
| Code owner, security contact, this file, the note at the top of `README.md` | `.github/CODEOWNERS`, `.github/SECURITY.md` |

Nothing else. A fix or a feature that is not about this fork's packaging goes
upstream as a pull request first, and arrives here by the next sync.

## Versions

Upstream's tags are in this repository as it left them; none of them is a
release here. A MathGaps release is the next patch number after the upstream
version it carries, and `CHANGELOG.md` says which upstream version that is. The
numbers therefore run ahead of upstream's: pin `mathgaps/apple` by this
repository's changelog, not by upstream's.

## Syncing with upstream

```shell
git remote add upstream https://github.com/AhmedOsman00/terraform-provider-apple.git
git fetch upstream
git rebase upstream/main        # drop the "publish as mathgaps/apple" commit if it conflicts
scripts/mathgaps-registry.sh    # and make it again
make generate && git diff --exit-code docs/
```

Then add a `CHANGELOG.md` section for the next version, naming the upstream
version it carries, and tag it.
