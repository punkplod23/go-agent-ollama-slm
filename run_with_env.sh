#!/bin/bash

# Export environment variables from .env file
export $(grep -v '^#' ../.env | tr -d '\r' | xargs)

# Build and run the Docker container
docker build -t go-agent-api .
docker run -p 8080:8080 \
  -e OPENWEBUIHOSTURL="$OPENWEBUIHOSTURL" \
  -e OPENWEBUIAPITOKEN="$OPENWEBUIAPITOKEN" \
  -e OPENWEBUIMODELNAME="$OPENWEBUIMODELNAME" \
  -e DVSAAPIURL="$DVSAAPIURL" \
  -e OPENALPRAPIURL="$OPENALPRAPIURL" \
  go-agent-api
