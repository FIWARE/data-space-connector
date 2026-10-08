#! /bin/bash
# Check the links of every Markdown file in the repository.
#
# Settings live in lychee.toml at the repository root: only relative links and
# anchors are checked, external URLs are skipped.
#
# lychee runs from its container, pinned like the Helm image in lint.sh, so the
# check behaves the same locally and in CI without installing anything.

set -uo pipefail

LYCHEE_IMAGE="lycheeverse/lychee:0.24.2"

docker run --rm -v "$(pwd):/input:ro" -w /input "$LYCHEE_IMAGE" \
    --config lychee.toml \
    --root-dir /input \
    --no-progress \
    '**/*.md'
