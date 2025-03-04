#!/bin/bash

# Define the port you want to kill the process on
PORT="61281"

# Get the PID of the process running on the port
PID=$(lsof -t -i :$PORT)

if [ ! -z "$PID" ]; then
  echo "Killing process on port $PORT (PID: $PID)"
  kill -9 $PID
else
  echo "No process found running on port $PORT"
fi

echo "Finished stopping process on port $PORT."