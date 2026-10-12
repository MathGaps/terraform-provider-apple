# MathGaps's fork

This is a fork of [AhmedOsman00/terraform-provider-apple](https://github.com/AhmedOsman00/terraform-provider-apple),
by AO Studio, under the Mozilla Public License 2.0 (`LICENSE`, unchanged). Most
of the provider is upstream's work.

The fork exists for two reasons. Upstream is published on the Terraform Registry
only, and OpenTofu resolves `ahmedosman00/apple` against `registry.opentofu.org`,
where it is not listed, so this fork releases to the OpenTofu Registry as
`mathgaps/apple`. And MathGaps needs resources upstream does not have yet, on
MathGaps's schedule, so they are built here first.

## What differs from upstream

| Change | Where |
|---|---|
| The registry address a configuration names: `mathgaps/apple` | everything `scripts/mathgaps-registry.sh` rewrites: `main.go`, `examples/`, `templates/`, `docs/`, `scripts/validate-examples.sh`, `README.md` |
| The release job runs in the `release` environment, which holds the signing key and admits only `v*` tags | `.github/workflows/release.yml` |
| GoReleaser makes the release a draft and the workflow publishes it once every asset is attached, so the OpenTofu registry never scans half a release | `.goreleaser.yml`, `.github/workflows/release.yml` |
| Code owner, security contact, this file, the note at the top of `README.md` | `.github/CODEOWNERS`, `.github/SECURITY.md` |
| Features MathGaps needed before upstream had them, each named in the `CHANGELOG.md` entry that released it | the files that change names |

A feature or fix MathGaps needs lands here first, in its own pull request, and
the `CHANGELOG.md` entry that releases it says it is the fork's. Once it is
released and working, the same change is offered upstream as a pull request, so
the two do not drift further than they must. When upstream merges it, the next
sync takes upstream's version and drops ours.

## Versions

Upstream's tags are in this repository as it left them; none of them is a
release here. Each MathGaps release counts from the previous MathGaps release,
never from upstream's number: one that only repackages upstream takes the next
patch number, and one that adds the fork's own resources takes the next minor
number, as any release with new features would. Every release is therefore
greater than the one before it. `CHANGELOG.md` names the upstream version each
release carries, separately from its own number, and what is the fork's. The
numbers run ahead of upstream's and mean different things: pin `mathgaps/apple`
by this repository's changelog, not by upstream's.

## Syncing with upstream

```shell
git remote add upstream https://github.com/AhmedOsman00/terraform-provider-apple.git
git fetch upstream
git rebase upstream/main        # drop the "publish as mathgaps/apple" commit if it conflicts
scripts/mathgaps-registry.sh    # and make it again
make generate && git diff --exit-code docs/
```

A fork-only change that upstream has not taken will conflict on the rebase
wherever upstream touched the same lines; resolve it toward upstream's code and
re-apply ours on top. Then add a `CHANGELOG.md` section for the next version,
naming the upstream version it carries, and tag it.
