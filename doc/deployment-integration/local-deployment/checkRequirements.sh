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

docker_reachable=false
if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
        docker_reachable=true
        echo "OK: Docker engine is reachable in the current context"
    else
        problem required "docker info failed. Start the configured Docker engine or check your Docker context and access permissions."
    fi
fi

# Memory and CPUs of the Docker engine, which runs the whole cluster. On macOS they are the ones of
# Docker's Linux VM, not of the host. The figures are the ones in LOCAL.MD (in GB, as written there).
if [ "$docker_reachable" = true ]; then
    resources=$(docker info --format '{{.MemTotal}} {{.NCPU}}' 2>/dev/null)
    memory=${resources%% *}
    cpus=${resources##* }
    case "$memory$cpus" in
        ''|*[!0-9]*)
            echo "NOTE: could not read the memory and CPUs of the Docker engine."
            ;;
        *)
            memory_gb=$(LC_ALL=C awk -v bytes="$memory" 'BEGIN { printf "%.1f", bytes / 1e9 }')
            if [ "$memory" -lt 16000000000 ]; then
                problem optional "the Docker engine has $memory_gb GB of memory, below the 16 GB the local deployment needs. Raise the memory of the Docker engine (on macOS, of its VM)."
            elif [ "$memory" -lt 24000000000 ]; then
                problem optional "the Docker engine has $memory_gb GB of memory; 24 GB or more are recommended for the local deployment."
            else
                echo "OK: Docker engine memory $memory_gb GB (recommended 24)"
            fi
            if [ "$cpus" -lt 2 ]; then
                problem optional "the Docker engine has $cpus CPU; with fewer than 2, the Java components take long to start and may fail their probes."
            else
                echo "OK: Docker engine CPUs $cpus"
            fi
            ;;
    esac
fi

# Host ports the k3s container publishes (see the k3s-maven-plugin configuration in pom.xml; the kube
# API port can be changed with -Dk3s.kubeApi.port). Connecting finds a listener of any user.
for port in 8080 8443 8888 6443; do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
        echo "OK: port $port is free"
    elif [ "$docker_reachable" = true ] &&
        [ -n "$(docker ps -q --filter 'name=^k3s-maven-plugin$' --filter "publish=$port" 2>/dev/null)" ]; then
        echo "OK: port $port is in use by the running local cluster"
    else
        problem optional "port $port is already in use. The local cluster publishes it on the host and cannot start while another process holds it."
    fi
done

# Endpoints such as https://keycloak-consumer.127.0.0.1.nip.io:8443 are opened from the host. Some
# resolvers drop answers that point to local addresses (DNS rebinding protection).
dns_name=tir.127.0.0.1.nip.io
resolved=
dns_checked=true
os=$(uname -s)
if [ "$os" = Darwin ] && command -v dscacheutil >/dev/null 2>&1; then
    resolved=$(dscacheutil -q host -a name "$dns_name" 2>/dev/null | awk '/^ip_address:/ { print $2; exit }')
elif command -v getent >/dev/null 2>&1; then
    resolved=$(getent hosts "$dns_name" 2>/dev/null | awk '{ print $1; exit }')
else
    dns_checked=false
    echo "NOTE: no resolver tool (getent or dscacheutil) found; $dns_name was not checked."
fi
if [ "$dns_checked" = true ]; then
    if [ "$resolved" = 127.0.0.1 ]; then
        echo "OK: $dns_name resolves to 127.0.0.1"
    else
        problem optional "$dns_name does not resolve to 127.0.0.1 on this host. Your DNS resolver may block answers that point to local addresses. The examples that go through the proxy (curl -x localhost:8888) still work; to open the endpoints from the host or a browser, allow nip.io in your resolver or use another DNS server."
    fi
fi

case "$os" in
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
