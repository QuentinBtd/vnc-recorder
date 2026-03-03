#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${GITHUB_BASE_REF:-}"
if [[ -z "$BASE_REF" ]]; then
	echo "GITHUB_BASE_REF is not set"
	exit 1
fi

git fetch --no-tags origin "$BASE_REF":"refs/remotes/origin/$BASE_REF"

CHANGED_FILES="$(git diff --name-only "origin/$BASE_REF...HEAD")"

if [[ -z "$CHANGED_FILES" ]]; then
	echo "No changed files detected."
	exit 0
fi

NON_EXEMPT_FILES="$(grep -Ev '(^CHANGELOG\.md$|^README\.md$|^docs/|^\.github/|^taskfile\.yaml$|^examples/|^scripts/|\.md$)' <<<"$CHANGED_FILES" || true)"

if [[ -z "$NON_EXEMPT_FILES" ]]; then
	echo "Only docs/CI/meta files changed; changelog update is optional."
	exit 0
fi

if ! grep -qE '^CHANGELOG\.md$' <<<"$CHANGED_FILES"; then
	echo "CHANGELOG.md must be updated in this pull request."
	echo
	echo "Changed files:"
	echo "$CHANGED_FILES"
	exit 1
fi

if ! grep -q '^## tip$' CHANGELOG.md; then
	echo "CHANGELOG.md must contain a '## tip' section for unreleased changes."
	exit 1
fi

echo "Changelog check passed."
