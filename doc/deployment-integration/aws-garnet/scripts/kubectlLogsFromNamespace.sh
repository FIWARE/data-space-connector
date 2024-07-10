#!/bin/bash

NAMESPACE="ips"

# Create a directory for pod logs if it doesn't exist
mkdir -p ./podLogs

# Associative array to store the last log timestamps for each pod
declare -A lastLogTimestamps

while true; do
  # Get the list of pods in the namespace
  PODS=$(kubectl get pods -n $NAMESPACE --no-headers -o custom-columns=":metadata.name")
  
  for POD in $PODS; do
    echo "Updating logs for pod: $POD"
    
    # Get the last timestamp we fetched logs up to
    lastTimestamp="${lastLogTimestamps[$POD]}"
    
    # Use 'kubectl logs' with '--since-time' to get logs since the last retrieval
    logs=$(kubectl logs -n $NAMESPACE $POD --since-time="$lastTimestamp")
    
    if [[ -z "$logs" ]]; then
      echo "No new logs found for pod $POD"
    else
      # Append the new logs to the pod's log file
      echo "$logs" >> "./podLogs/$POD.log"
      echo "----------------------------------" >> "./podLogs/$POD.log"
      
      # Update the last log timestamp for this pod
      lastLogTimestamps[$POD]=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    fi
  done
  
  # Sleep for a few seconds before checking for new logs and pods again
  sleep 3
done
