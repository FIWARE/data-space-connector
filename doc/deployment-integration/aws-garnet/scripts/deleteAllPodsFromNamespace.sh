#!/bin/bash

# Set the fixed namespace
namespace="ips"

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    echo "kubectl not found. Please install kubectl before running this script."
    exit 1
fi

# Confirm with the user before proceeding
read -p "This will delete all pods in the '$namespace' namespace. Are you sure? (y/n): " confirm
if [[ $confirm != "y" ]]; then
    echo "Operation canceled."
    exit 0
fi

# Delete all pods in the specified namespace
kubectl delete pods --namespace="$namespace" --all

echo "Deleting pods in the '$namespace' namespace..."
