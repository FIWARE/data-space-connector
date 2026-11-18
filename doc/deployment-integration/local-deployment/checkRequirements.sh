#!/bin/bash
# Compatible with Bash 3.2 and later (including the macOS system shell).
set -u

tools=(docker kubectl curl jq yq mvn java javac)
# Define required versions
required_versions=(
    "27.0.0"
    "1.31.0"
    "7.81.0"
    "1.6"
    "4.45.0"
    "3.6.3"
    "17"
    "17"
)

# Define installation links
install_links=(
    "https://docs.docker.com/get-docker/"
    "https://kubernetes.io/docs/tasks/tools/install-kubectl/"
    "https://curl.se/download.html"
    "https://stedolan.github.io/jq/download/"
    "https://mikefarah.gitbook.io/yq/"
    "https://maven.apache.org/install.html"
    "https://adoptium.net/installation/"
    "https://adoptium.net/installation/"
)

failures=0

problem() {
    if [ "$1" = required ]; then
        failures=$((failures + 1))
        echo "ERROR: $2"
    else
        echo "WARNING: $2"
    fi
}

# Compare numeric components, treating omitted minor/patch components as zero.
# Avoid sort -V, which is unavailable on some supported macOS versions.
version_ge() {
    awk -v installed="$1" -v required="$2" 'BEGIN {
        split(installed, a, "."); split(required, b, ".")
        for (i = 1; i <= 3; i++) {
            if (a[i] + 0 > b[i] + 0) exit 0
            if (a[i] + 0 < b[i] + 0) exit 1
        }
        exit 0
    }'
}

# Check tools
for i in "${!tools[@]}"; do
    tool="${tools[$i]}"
    requirement=required
    case "$tool" in curl|jq|yq) requirement=optional ;; esac
    if ! command -v "$tool" >/dev/null 2>&1; then
        problem "$requirement" "$tool is not installed. Install it here: ${install_links[$i]}"
    else
        version_args=(--version)
        case "$tool" in
            kubectl) version_args=(version --client); version_pattern='^Client Version: v?[0-9]' ;;
            java) version_args=(-version); version_pattern='^(openjdk|java) (version )?"?[0-9]' ;;
            javac) version_args=(-version); version_pattern='^javac [0-9]' ;;
            docker) version_pattern='^Docker version [0-9]' ;;
            mvn) version_pattern='^Apache Maven [0-9]' ;;
            curl) version_pattern='^curl [0-9]' ;;
            jq) version_pattern='^jq-[0-9]' ;;
            yq) version_pattern='^yq .*version v?[0-9]' ;;
        esac
        if ! output=$("$tool" "${version_args[@]}" 2>&1); then
            problem "$requirement" "$tool could not run its version command. Check its installation: ${install_links[$i]}"
            continue
        fi
        # Ignore startup diagnostics (e.g. JAVA_TOOL_OPTIONS) and Maven's ANSI colors.
        version_line=$(printf '%s\n' "$output" | awk '{ gsub(/\033\[[0-9;]*m/, ""); print }' |
            grep -E "$version_pattern" | head -n1)
        if [ "$tool" = yq ]; then
            version_line=${version_line##* version }
        fi
        current_version=$(printf '%s\n' "$version_line" | grep -oE '[0-9]+(\.[0-9]+){0,2}' | head -n1)

        if [ -z "$current_version" ]; then
            problem "$requirement" "Could not determine version for $tool. Install or update it: ${install_links[$i]}"
        elif ! version_ge "$current_version" "${required_versions[$i]}"; then
            problem "$requirement" "$tool version $current_version is installed but ${required_versions[$i]} or higher is required. Update it here: ${install_links[$i]}"
        else
            echo "OK: $tool $current_version (minimum ${required_versions[$i]})"
        fi
    fi
done

if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
        echo "OK: Docker engine is reachable in the current context"
    else
        problem required "docker info failed. Start the configured Docker engine or check your Docker context and access permissions."
    fi
fi

case "$(uname -s)" in
    Linux)
        if [ -d /sys/module/br_netfilter ]; then
            echo "OK: br_netfilter is available in the host kernel"
        else
            echo "WARNING: br_netfilter was not found in the host kernel. If Docker uses this kernel, check whether sudo modprobe br_netfilter is needed. A remote Docker engine uses its own kernel."
        fi
        ;;
    Darwin)
        echo "NOTE: k3s networking uses the Docker engine's Linux VM; no macOS host kernel-module check is needed."
        ;;
    *)
        echo "WARNING: host kernel checks are only supported on Linux and macOS."
        ;;
esac

if [ "$failures" -ne 0 ]; then
    echo "Preflight failed: $failures required check(s) failed."
    exit 1
fi
echo "Preflight passed. Cluster startup has not been tested."
