#!/bin/bash

# Define the ports you want to kill the processes on
PORTS=("61280" "61281")

# Loop through each port and find the corresponding process
for PORT in "${PORTS[@]}"; do
  # Get the PID of the process running on the port
  PID=$(lsof -t -i :$PORT)

  if [ ! -z "$PID" ]; then
    echo "Killing process on port $PORT (PID: $PID)"
    kill -9 $PID
  else
    echo "No process found running on port $PORT"
  fi
done

echo "Finished stopping processes on specified ports."
