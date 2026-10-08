#!/usr/bin/env bash
# The whole of what MathGaps's fork changes in upstream's sources: the registry
# address a configuration names, ahmedosman00/apple -> mathgaps/apple. It is
# mechanical so that a rebase onto upstream can drop the commit it made and run
# it again (FORK.md). docs/ is generated from examples/ and templates/, which
# this rewrites too, so the result is what `make generate` would produce.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

git grep -lz -e 'ahmedosman00' -e 'published on the Terraform Registry' -- main.go examples templates docs scripts README.md ':!scripts/mathgaps-registry.sh' |
  xargs -0 perl -pi -e '
    s{https://registry\.terraform\.io/providers/ahmedosman00/apple/latest}{https://search.opentofu.org/provider/mathgaps/apple/latest}g;
    s{registry\.terraform\.io/ahmedosman00/apple(?=")}{registry.opentofu.org/mathgaps/apple}g if $ARGV eq "main.go";
    s{ahmedosman00/}{mathgaps/}g;
    s{published on the Terraform Registry as}{published on the OpenTofu Registry as}g;
    s{The Terraform Registry address this provider is published under}{The OpenTofu Registry address MathGaps'"'"'s fork is published under}g;
    s{namespace "AhmedOsman00" is written here as "ahmedosman00"}{namespace "MathGaps" is written here as "mathgaps"}g;
  '
