#! /bin/bash
# Lint every chart under ./charts.
#
# Only directories that hold a Chart.yaml are linted: ./charts also contains a
# README.md, and handing that to `helm lint` fails the run with
# "No chart found for linting (missing Chart.yaml)".
#
# Helm is pinned to the version the integration tests render with. The 2.9.0
# image used before predates Go 1.11, so its template parser did not know
# variable assignment (`{{- $x = ... }}`) and rejected charts that render fine
# with every Helm the project otherwise uses, with
# "unexpected "=" in operand".
#
# Subchart dependencies are not fetched here - a missing charts/ directory is a
# lint warning, not an error - so linting stays independent of the registries.

set -uo pipefail

HELM_IMAGE="alpine/helm:3.17.3"

status=0
for chart in ./charts/*/; do
    if [ ! -f "${chart}Chart.yaml" ]; then
        continue
    fi
    # every chart is linted before the result is reported, so one failure does
    # not hide the state of the others
    if ! docker run --rm -v "$(pwd):/apps" "$HELM_IMAGE" lint "$chart"; then
        status=1
    fi
done

exit $status
