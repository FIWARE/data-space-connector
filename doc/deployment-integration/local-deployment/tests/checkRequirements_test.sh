#!/bin/bash
# Exercise direct execution, tool diagnostics and exit statuses without real services.
set -eu

CHECKER="$(cd "$(dirname "$0")/.." && pwd)/checkRequirements.sh"
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT
BIN="$TEST_DIR/bin"
mkdir "$BIN"
for tool in awk grep head uname; do
    ln -s "$(command -v "$tool")" "$BIN/$tool"
done

cat > "$TEST_DIR/tool" <<'TOOL'
#!/bin/sh
mode=${CHECKER_FIXTURE_MODE:-healthy}
if [ "$mode" = startup-noise ]; then
    case "${0##*/}" in
        java|javac|mvn) echo 'Picked up JAVA_TOOL_OPTIONS: -Xmx16m' >&2 ;;
    esac
fi
case "${0##*/}" in
    docker)
        case "$*" in
            --version) echo 'Docker version 27.0.0, build fixture' ;;
            info) [ "$mode" != daemon-down ] ;;
            'info --format'*)
                case "$mode" in
                    small-engine) echo '8000000000 1' ;;
                    medium-engine) echo '20000000000 8' ;;
                    *) echo '32000000000 8' ;;
                esac
                ;;
            *) exit 42 ;;
        esac
        ;;
    getent)
        [ "$mode" = dns-blocked ] && exit 2
        echo '127.0.0.1       tir.127.0.0.1.nip.io'
        ;;
    dscacheutil)
        [ "$mode" = dns-blocked ] && exit 0
        printf 'name: tir.127.0.0.1.nip.io\nip_address: 127.0.0.1\n\n'
        ;;
    kubectl)
        [ "$*" = 'version --client' ] || exit 42
        if [ "$mode" = old-kubectl ]; then
            echo 'Client Version: v1.9.0'
        else
            echo 'Client Version: v1.31.0'
        fi
        ;;
    mvn)
        [ "$*" = --version ] || exit 42
        case "$mode" in
            broken-mvn) echo 'Apache Maven 3.9.0'; exit 7 ;;
            unknown-mvn) echo 'No version available' ;;
            *) printf '\033[1mApache Maven 3.6.3\033[0m\n' ;;
        esac
        ;;
    java)
        [ "$*" = -version ] || exit 42
        if [ "$mode" = old-java ]; then
            echo 'java version "1.8.0_442"' >&2
        else
            echo 'openjdk version "17.0.1"' >&2
        fi
        ;;
    javac)
        [ "$*" = -version ] || exit 42
        echo 'javac 17' >&2
        ;;
    curl) echo 'curl 7.81.0 (fixture)' ;;
    jq)
        if [ "$mode" = old-jq ]; then echo jq-1.5; else echo jq-1.6; fi
        ;;
    yq) echo 'yq (https://github.com/mikefarah/yq/) version v4.45.0' ;;
esac
TOOL
chmod +x "$TEST_DIR/tool"
for tool in docker kubectl mvn java javac curl jq yq getent dscacheutil; do
    ln -s "$TEST_DIR/tool" "$BIN/$tool"
done

run_case() {
    local name="$1" mode="$2" expected="$3" pattern="$4" status
    if CHECKER_FIXTURE_MODE="$mode" PATH="$BIN" "$CHECKER" >"$TEST_DIR/$name.log" 2>&1; then
        status=0
    else
        status=$?
    fi
    if [ "$status" -ne "$expected" ] || ! grep -Fq "$pattern" "$TEST_DIR/$name.log"; then
        echo "FAIL: $name (exit $status, expected $expected; expected text: $pattern)"
        cat "$TEST_DIR/$name.log"
        exit 1
    fi
    echo "PASS: $name"
}

[ -x "$CHECKER" ] || { echo 'Checker must be executable'; exit 1; }
run_case healthy healthy 0 'Preflight passed.'
for tool in docker kubectl mvn java javac curl jq yq; do
    grep -Fq "OK: $tool " "$TEST_DIR/healthy.log"
done
case "$(uname -s)" in
    Darwin)
        grep -Fq "Docker engine's Linux VM" "$TEST_DIR/healthy.log"
        if grep -Eq 'modprobe|lsmod|declare:' "$TEST_DIR/healthy.log"; then
            echo 'FAIL: Linux host commands or unsupported shell syntax on macOS'
            exit 1
        fi
        ;;
    Linux) grep -Fq br_netfilter "$TEST_DIR/healthy.log" ;;
esac
grep -Fq 'OK: Docker engine memory 32.0 GB' "$TEST_DIR/healthy.log"
grep -Fq 'OK: Docker engine CPUs 8' "$TEST_DIR/healthy.log"
grep -Fq 'OK: tir.127.0.0.1.nip.io resolves to 127.0.0.1' "$TEST_DIR/healthy.log"
# The ports depend on the machine running the test: only check that all four are reported.
for port in 8080 8443 8888 6443; do
    grep -Eq "(OK|WARNING): port $port " "$TEST_DIR/healthy.log"
done
run_case small-engine small-engine 0 'WARNING: the Docker engine has 8.0 GB of memory, below the 16 GB'
grep -Fq 'WARNING: the Docker engine has 1 CPU' "$TEST_DIR/small-engine.log"
run_case medium-engine medium-engine 0 'WARNING: the Docker engine has 20.0 GB of memory; 24 GB or more'
run_case dns-blocked dns-blocked 0 'WARNING: tir.127.0.0.1.nip.io does not resolve to 127.0.0.1'
run_case old-kubectl old-kubectl 1 'kubectl version 1.9.0 is installed but 1.31.0'
run_case old-java old-java 1 'java version 1.8.0 is installed but 17'
run_case daemon-down daemon-down 1 'ERROR: docker info failed.'
run_case broken-version broken-mvn 1 'ERROR: mvn could not run its version command.'
run_case unknown-version unknown-mvn 1 'ERROR: Could not determine version for mvn.'
run_case optional-outdated old-jq 0 'WARNING: jq version 1.5'
run_case startup-noise startup-noise 0 'Preflight passed.'
grep -Fq 'OK: java 17.0.1' "$TEST_DIR/startup-noise.log"
grep -Fq 'OK: javac 17 ' "$TEST_DIR/startup-noise.log"
grep -Fq 'OK: mvn 3.6.3' "$TEST_DIR/startup-noise.log"

mv "$BIN/java" "$TEST_DIR/java"
mv "$BIN/javac" "$TEST_DIR/javac"
run_case missing-jdk healthy 1 'Preflight failed: 2 required check(s) failed.'
grep -Fq 'ERROR: java is not installed.' "$TEST_DIR/missing-jdk.log"
grep -Fq 'ERROR: javac is not installed.' "$TEST_DIR/missing-jdk.log"
mv "$TEST_DIR/java" "$BIN/java"
mv "$TEST_DIR/javac" "$BIN/javac"

for tool in curl jq yq; do mv "$BIN/$tool" "$TEST_DIR/$tool"; done
run_case optional-missing healthy 0 'Preflight passed.'
for tool in curl jq yq; do
    grep -Fq "WARNING: $tool is not installed." "$TEST_DIR/optional-missing.log"
done

echo 'All requirements-checker tests passed.'
