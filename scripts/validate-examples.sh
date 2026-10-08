#!/usr/bin/env bash
# Copyright (c) AO Studio
# SPDX-License-Identifier: MPL-2.0

# Run `terraform validate` over every directory under examples/.
#
# `make generate` only runs `terraform fmt`, which catches syntax but not a
# `settings { ... }` block written against a ListNestedAttribute, an attribute
# the schema does not have, or a value a validator rejects. Those only surface
# under `validate`, which needs the provider installed.
#
# The working tree is built here and installed into a throwaway filesystem
# mirror, so the examples validate against the schema in this checkout rather
# than the last release on the Registry. dev_overrides is deliberately not used:
# it makes `terraform init` refuse to run, and these directories need init.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROVIDER_ADDR="registry.terraform.io/mathgaps/apple"
# Must satisfy the version constraint the examples pin.
PROVIDER_VERSION="0.1.0"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

MIRROR="$WORK/mirror/$PROVIDER_ADDR/$PROVIDER_VERSION/$(go env GOOS)_$(go env GOARCH)"
mkdir -p "$MIRROR"

echo "==> Building the provider into a local mirror"
go build -o "$MIRROR/terraform-provider-apple_v$PROVIDER_VERSION" "$REPO_ROOT"

cat > "$WORK/terraformrc" <<EOF
provider_installation {
  filesystem_mirror {
    path    = "$WORK/mirror"
    include = ["registry.terraform.io/mathgaps/*"]
  }
  direct {
    exclude = ["registry.terraform.io/mathgaps/*"]
  }
}
EOF
export TF_CLI_CONFIG_FILE="$WORK/terraformrc"
export TF_IN_AUTOMATION=1

failed=()
while read -r dir; do
  name="${dir//\//_}"
  staged="$WORK/staged/$name"
  mkdir -p "$staged"
  cp "$REPO_ROOT/$dir"/*.tf "$staged/"

  # tfplugindocs examples carry no terraform block of their own, so the source
  # address has to be supplied for init to resolve the provider.
  if ! grep -qs required_providers "$staged"/*.tf; then
    cat > "$staged/zz_required_providers_shim.tf" <<EOF
terraform {
  required_providers {
    apple = {
      source = "$PROVIDER_ADDR"
    }
  }
}
EOF
  fi

  echo "==> $dir"
  if (cd "$staged" && terraform init -backend=false -input=false -no-color > /dev/null && terraform validate -no-color); then
    :
  else
    failed+=("$dir")
  fi
done < <(cd "$REPO_ROOT" && find examples -type f -name '*.tf' -exec dirname {} \; | sort -u)

if [ ${#failed[@]} -ne 0 ]; then
  echo
  echo "terraform validate failed in ${#failed[@]} example director$([ ${#failed[@]} -eq 1 ] && echo y || echo ies):"
  printf '  %s\n' "${failed[@]}"
  exit 1
fi

echo
echo "All example directories validate."
