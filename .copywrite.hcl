schema_version = 1

project {
  license          = "MPL-2.0"
  copyright_year   = 2025
  copyright_holder = "AO Studio"

  header_ignore = [
    # Terraform examples used within documentation (prose)
    "examples/**",

    # Generated provider documentation
    "docs/**",

    # GitHub issue template configuration
    ".github/ISSUE_TEMPLATE/*.yml",

    # golangci-lint tooling configuration
    ".golangci.yml",

    # GoReleaser tooling configuration
    ".goreleaser.yml",

    # MathGaps's fork (FORK.md): not AO Studio's file
    "scripts/mathgaps-registry.sh",
  ]
}
