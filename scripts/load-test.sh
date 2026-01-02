#!/bin/bash
set -e

# Default values
TARGET_URL=${TARGET_URL:-"http://localhost:8080"}
API_KEY=${API_KEY:-"9d78136e-308b-49fd-967f-e62b9b91f1d8:load-test-secret"}
SCENARIO=$1

if [ -z "$SCENARIO" ]; then
    echo "Usage: $0 <scenario> [target_url] [api_key]"
    echo "Available scenarios: smoke, load, stress, soak"
    exit 1
fi

if [ ! -f "test/load/scenarios/$SCENARIO.js" ]; then
    echo "Error: Scenario '$SCENARIO' not found in test/load/scenarios/"
    exit 1
fi


# Check if k6 is installed locally
if command -v k6 &> /dev/null; then
    echo "Using local k6 installation..."
    k6 run \
        -e TARGET_URL="$TARGET_URL" \
        -e API_KEY="$API_KEY" \
        "test/load/scenarios/$SCENARIO.js"
elif command -v docker &> /dev/null; then
    echo "Local k6 not found, using Docker image..."
    
    # Get absolute path to the project root
    REPO_ROOT=$(pwd)

    # Use host networking to allow access to localhost
    docker run --rm -i \
        --network host \
        -v "$REPO_ROOT/test/load:/test/load" \
        -e TARGET_URL="$TARGET_URL" \
        -e API_KEY="$API_KEY" \
        grafana/k6:latest run "/test/load/scenarios/$SCENARIO.js"
else
    echo "Error: Neither 'k6' nor 'docker' commands were found."
    exit 1
fi
