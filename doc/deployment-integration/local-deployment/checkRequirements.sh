#!/bin/bash

# Define required versions
declare -A required_versions
required_versions=(
    [docker]="27.0.0"
    [kubectl]="1.31.0"
    [curl]="7.81.0"
    [jq]="1.6"
    [yq]="4.45.0"
    [mvn]="3.6.3"
)

# Define installation links
declare -A install_links
install_links=(
    [docker]="https://docs.docker.com/get-docker/"
    [kubectl]="https://kubernetes.io/docs/tasks/tools/install-kubectl/"
    [curl]="https://curl.se/download.html"
    [jq]="https://stedolan.github.io/jq/download/"
    [yq]="https://mikefarah.gitbook.io/yq/"
    [mvn]="https://maven.apache.org/install.html"
)

# Function to compare versions
version_ge() {
    printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1 | grep -q "$2"
}

# Check tools
for tool in "${!required_versions[@]}"; do
    if ! command -v "$tool" &>/dev/null; then
        echo "$tool is not installed. Install it here: ${install_links[$tool]}"
    else
        case $tool in
            kubectl)
                current_version=$(kubectl version --client 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n1)
                ;;
            jq)
                current_version=$(jq --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | head -n1)
                ;;
            *)
                current_version=$($tool --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n1)
                ;;
        esac
        
        if [ -z "$current_version" ]; then
            echo "Could not determine version for $tool. Install or update it: ${install_links[$tool]}"
        elif ! version_ge "$current_version" "${required_versions[$tool]}"; then
            echo "$tool version $current_version is installed but ${required_versions[$tool]} or higher is required. Update it here: ${install_links[$tool]}"
        fi
    fi
done

# Check if br_netfilter module is enabled
if lsmod | grep -q br_netfilter; then
    echo
else
    echo "br_netfilter module is not enabled, but required for k3s networking. Enable it using: sudo modprobe br_netfilter"
fi