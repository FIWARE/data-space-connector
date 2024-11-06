#!/bin/bash

# Script to check if the basic requirements for a local deployment of the Data Space Connector are met

# Command for starting a minimal k3s cluster
k3s_command="docker run --privileged --volume=k3stest:/var/lib/rancher/k3s/agent --detach --publish=6443:6443 docker.io/rancher/k3s:latest server --node-name=k3s --https-listen-port=6443 --disable-cloud-controller --disable-network-policy --disable=metrics-server --disable-helm-controller --disable=local-storage --disable=traefik"

check_tool() {
    local tool_name=$1
    local test_command=$2

    # Check if the tool is installed
    if ! command -v $tool_name &> /dev/null; then
        echo "$tool_name is not installed."
        exit 1
    fi

    # Run the test command and check for errors
     if ! output=$($test_command 2>&1); then
        echo "$tool_name is installed but failed to run."
        echo "Error output:"
        echo "$output"
        exit 1
    fi

    echo "$tool_name is installed and works correctly."
    return 0
}

wait_till_running() {
    local containerId=$1
    sleep 10

    end=$((SECONDS+60))

    while [ $SECONDS -lt $end ]; do
        local state=$(docker inspect -f '{{.State.Status}}' $containerId)
        if [[ "$state" = "running" || "$state" = "dead" ]]; then
            break
        else
            echo "Waiting for Container, current state $(state). Checking again in 5 seconds..."
            sleep 5
        fi
    done
}
echo "Testing system for requirements running the FIWARE Data Space Connector Local Deployment"

# Check if the script is running on Linux
if [[ "$(uname)" != "Linux" ]]; then
    echo "Local Deployment is currently only tested on Linux."
    exit 1
fi

# Check Maven
check_tool "mvn" "mvn --version"

# Check Docker
check_tool "docker" "docker --version"

# Check if k3s can be run
echo "Attempting to start a k3s container"
containerId=""
if output=$($k3s_command 2>&1); then
    containerId=$output
    echo "k3s container starting with ID: $containerId"
else
    echo "Failed to start k3s container."
    echo "Error output:"
    echo "$output"
    exit 1
fi

wait_till_running $containerId

if [ "$(docker inspect -f '{{.State.Status}}' $containerId)" = "running" ]; then
    echo "Container is running."
else
    echo "Container is not running."    
    if docker logs $containerId 2>&1 | grep -q 'inotify_init: too many open files'; then
        echo "inotify limit is to low, check your inotify.max_user_instances config and set to a higher value"
        exit 1
    else
        echo "Check logs for error hints:"
        docker logs $containerId | tail -n20
        exit 1
    fi
fi

# Copy kubeconfig file from container
docker cp $containerId:/etc/rancher/k3s/k3s.yaml .

# Let kubectl use copied config
export KUBECONFIG=$(pwd)/k3s.yaml

# Check Kubectl
check_tool "kubectl" "kubectl get nodes"
echo "Kubectl is able to access the local kubernetes cluster successfully!"


# cleanup
docker stop $containerId
docker rm $containerId

echo "Test complete"
exit 0
