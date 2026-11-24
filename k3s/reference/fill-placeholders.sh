#!/bin/bash
# Fills in the <placeholders> of a reference profile (charts/*/examples) the way a user does, with
# the values of one environment, so that the integration tests deploy the files as published.
#
#   fill-placeholders.sh <values.env> <profile.yaml> <output.yaml>
#
# <values.env> holds one `placeholder=value` per line (without the angle brackets); empty lines and
# lines starting with # are ignored. The script fails if a placeholder is left in the output,
# outside of comments.
set -euo pipefail

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <values.env> <profile.yaml> <output.yaml>" >&2
    exit 2
fi
env_file=$1
profile=$2
output=$3

mkdir -p "$(dirname "$output")"
cp "$profile" "$output.tmp"
while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    name=${line%%=*}
    value=${line#*=}
    # | is the sed delimiter: the values are hosts, URLs and DIDs, which never contain it
    sed -i.bak "s|<${name}>|${value}|g" "$output.tmp"
done < "$env_file"
rm -f "$output.tmp.bak"

# a placeholder is <lowercase-words>; the ones in comments (optional settings) may stay
if left=$(grep -n -v '^[[:space:]]*#' "$output.tmp" | grep '<[a-z][a-z-]*>'); then
    echo "Placeholders without a value in $env_file, for $profile:" >&2
    echo "$left" >&2
    rm -f "$output.tmp"
    exit 1
fi
mv "$output.tmp" "$output"
