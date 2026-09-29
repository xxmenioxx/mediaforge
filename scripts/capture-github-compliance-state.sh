#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'GitHub compliance snapshot failed: %s\n' "$*" >&2
  exit 1
}

[[ -n "${GITHUB_TOKEN_CODEX:-}" ]] || fail "GITHUB_TOKEN_CODEX is required for read-only GitHub API access"
[[ $# -eq 1 ]] || fail "usage: $0 /path/to/github-compliance-state.json"

output_path="$1"
[[ ! -e "$output_path" ]] || fail "refusing to overwrite existing snapshot: $output_path"
output_parent="$(dirname "$output_path")"
mkdir -p "$output_parent"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/mvforge-github-compliance.XXXXXX")"
trap 'rm -f "$temporary_dir"/*; rmdir "$temporary_dir"' EXIT

GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api repos/xxmenioxx/mediaforge > "$temporary_dir/repository.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api --paginate --slurp repos/xxmenioxx/mediaforge/releases > "$temporary_dir/releases.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api --paginate --slurp repos/xxmenioxx/mediaforge/git/matching-refs/tags/ > "$temporary_dir/tags.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api repos/xxmenioxx/mediaforge/actions/variables > "$temporary_dir/variables.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api '/users/xxmenioxx/packages/container/mvforge-backend' > "$temporary_dir/backend-package.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api --paginate --slurp '/users/xxmenioxx/packages/container/mvforge-backend/versions?per_page=100' > "$temporary_dir/backend-versions-pages.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api '/users/xxmenioxx/packages/container/mvforge-web' > "$temporary_dir/web-package.json"
GH_TOKEN="$GITHUB_TOKEN_CODEX" gh api --paginate --slurp '/users/xxmenioxx/packages/container/mvforge-web/versions?per_page=100' > "$temporary_dir/web-versions-pages.json"

jq -n \
  --arg captured_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --slurpfile repository "$temporary_dir/repository.json" \
  --slurpfile releases "$temporary_dir/releases.json" \
  --slurpfile tags "$temporary_dir/tags.json" \
  --slurpfile variables "$temporary_dir/variables.json" \
  --slurpfile backend_package "$temporary_dir/backend-package.json" \
  --slurpfile backend_pages "$temporary_dir/backend-versions-pages.json" \
  --slurpfile web_package "$temporary_dir/web-package.json" \
  --slurpfile web_pages "$temporary_dir/web-versions-pages.json" \
  '
  def records($pages): [$pages[][] | {
    id,
    digest: .name,
    created_at,
    updated_at,
    tags: (.metadata.container.tags // [])
  }];
  def package_record($package; $pages):
    (records($pages)) as $versions |
    {
      exists: true,
      package_type: $package[0].package_type,
      visibility: $package[0].visibility,
      html_url: $package[0].html_url,
      version_count: ($versions | length),
      tagged_version_count: ([$versions[] | select((.tags | length) > 0)] | length),
      untagged_version_count: ([$versions[] | select((.tags | length) == 0)] | length),
      unique_tag_count: ([$versions[].tags[]] | unique | length),
      versions: $versions
    };
  {
    captured_at: $captured_at,
    repository: $repository[0].full_name,
    repository_visibility: (if $repository[0].private then "private" else "public" end),
    current_releases: {count: ([$releases[0][][]] | length), endpoint: "GET /repos/xxmenioxx/mediaforge/releases"},
    current_git_tag_refs: {count: ([$tags[0][][]] | length), endpoint: "GET /repos/xxmenioxx/mediaforge/git/matching-refs/tags/"},
    actions_variables: {count: $variables[0].total_count, names: [$variables[0].variables[]?.name]},
    api_field_limits: {
      version_digest: "The API exposes the digest in the version name field (sha256:...). No separate digest field is returned.",
      container_metadata: "Version metadata exposes tags; it does not expose platform, SBOM, provenance, or attestation fields."
    },
    packages: {
      "mvforge-backend": package_record($backend_package; $backend_pages[0]),
      "mvforge-web": package_record($web_package; $web_pages[0])
    }
  }' > "$output_path"

printf 'Wrote read-only GitHub compliance snapshot: %s\n' "$output_path"
