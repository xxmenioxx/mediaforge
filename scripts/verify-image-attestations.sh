#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'image attestation check failed: %s\n' "$*" >&2
  exit 1
}

[[ $# -eq 2 ]] || fail "usage: $0 IMAGE_NAME sha256:DIGEST"
image="$1"
digest="$2"
[[ "$image" =~ ^ghcr\.io/xxmenioxx/mvforge-(backend|web)$ ]] || fail "unexpected image name: $image"
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "invalid image digest: $digest"
command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"

index_json="$(docker buildx imagetools inspect --raw "${image}@${digest}")" \
  || fail "could not inspect image index ${image}@${digest}"
image_digests="$(jq -r '
  [.manifests[]? | select(.annotations["vnd.docker.reference.type"] != "attestation-manifest") | .digest] | .[]
' <<<"$index_json")"
[[ -n "$image_digests" ]] || fail "image index contains no image manifests"

image_digest_json="$(jq -Rsc 'split("\n") | map(select(length > 0))' <<<"$image_digests")"
attestation_digests="$(jq -r --argjson image_digests "$image_digest_json" '
  [.manifests[]? | select(.annotations["vnd.docker.reference.type"] == "attestation-manifest")
   | select(.annotations["vnd.docker.reference.digest"] as $subject
       | ($image_digests | index($subject)) != null)
   | .digest] | unique | .[]
' <<<"$index_json")"
[[ -n "$attestation_digests" ]] || fail "no BuildKit attestation manifests reference the image manifests"

has_spdx=false
has_slsa=false
while IFS= read -r attestation_digest; do
  manifest="$(docker buildx imagetools inspect --raw "${image}@${attestation_digest}")" \
    || fail "could not inspect attestation ${attestation_digest}"
  predicate_types="$(jq -r '.layers[]?.annotations["in-toto.io/predicate-type"] // empty' <<<"$manifest")"
  if grep -Fxq 'https://spdx.dev/Document' <<<"$predicate_types"; then
    has_spdx=true
  fi
  if grep -Eq '^https://slsa\.dev/provenance/(v[0-9]+(\.[0-9]+)?|v1)$' <<<"$predicate_types"; then
    has_slsa=true
  fi
done <<<"$attestation_digests"

[[ "$has_spdx" == true ]] || fail "no SPDX SBOM predicate was found for the image digest"
[[ "$has_slsa" == true ]] || fail "no SLSA provenance predicate was found for the image digest"
printf 'Verified BuildKit SPDX SBOM and SLSA provenance for %s@%s.\n' "$image" "$digest"
