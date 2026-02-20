#!/bin/bash

delete_list=(
  "regular_reveal_order.json"
  "broadcast_trackers.json"
  "reveal_orders.json"
  "registered_nodes.json"
  "commits.json"
  "leader_commits.json"
  "peerNodeInfo.json"
)

dry_run=false
if [ "$1" = "--dry-run" ]; then
  dry_run=true
fi

for file in "${delete_list[@]}"; do
  if [ -f "$file" ]; then
    if $dry_run; then
      echo "[Dry Run] Would delete: $file"
    else
      echo "Deleting $file"
      rm "$file"
    fi
  fi
done