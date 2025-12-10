#!/bin/bash

# --- Load Environment Variables ---

ENV_FILE=".env"

if [ ! -f "$ENV_FILE" ]; then
    echo "Error: Environment file not found at '$ENV_FILE'" >&2
    exit 1
fi

# Use 'allexport' to export all variables sourced from the .env file.
# This is more robust than the 'grep | xargs' method.
# We use process substitution with sed to strip carriage returns ('\r') for Windows compatibility.
set -a
source <(sed 's/\r$//' "$ENV_FILE")
set +a

# --- Environment Variable Validation ---

# List of required environment variables
REQUIRED_VARS=(
  "OPENWEBUIHOSTURL"
  "OPENWEBUIAPITOKEN"
  "OPENWEBUIMODELNAME"
  "DVSAAPIURL"
  "OPENALPRAPIURL"
)

for var in "${REQUIRED_VARS[@]}"; do
  if [ -z "${!var}" ]; then
    echo "Error: Required environment variable '$var' is not set or is empty." >&2
    echo "Please ensure it is defined in your '$ENV_FILE' file." >&2
    exit 1
  fi
done

# --- (Optional) Connecting to a Service in Kubernetes ---
# If OPENWEBUIHOSTURL points to a service in Kubernetes, you must port-forward it.
# Example: kubectl port-forward service/open-webui 9090:8080 -n <your-namespace>
# Then set OPENWEBUIHOSTURL=http://localhost:9090 in your ../.env file.

# Build and run the Docker container
echo "Building Docker image 'go-agent-api'..."
docker build -t go-agent-api .
 
CONTAINER_NAME="go-agent-api-container"
 
# Stop and remove the container if it already exists to prevent conflicts
echo "Stopping and removing old container instance '$CONTAINER_NAME'..."
docker rm -f $CONTAINER_NAME 2>/dev/null || true
 
echo "Starting new container '$CONTAINER_NAME'..."
docker run --name $CONTAINER_NAME -p 8080:8080 \
  -e OPENWEBUIHOSTURL="$OPENWEBUIHOSTURL" \
  -e OPENWEBUIAPITOKEN="$OPENWEBUIAPITOKEN" \
  -e OPENWEBUIMODELNAME="$OPENWEBUIMODELNAME" \
  -e DVSAAPIURL="$DVSAAPIURL" \
  -e OPENALPRAPIURL="$OPENALPRAPIURL" \
  go-agent-api
