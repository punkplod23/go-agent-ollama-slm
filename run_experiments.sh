#!/bin/bash

# Configuration
BASE_URL="http://localhost:8080/api/v1"
KNOWLEDGE_ID="REPLACE_WITH_VALID_KNOWLEDGE_ID" # Update this to test file uploads
TEST_FILE="test.md"

echo "========================================"
echo "Running API Experiments"
echo "========================================"

# 1. Test Chat Endpoint
echo -e "\n[1] Testing Chat Endpoint..."
curl -X POST "$BASE_URL/chat" \
     -H "Content-Type: application/json" \
     -d '{"prompt": "What is the status of the system?"}'

# 2. Test Process Base64 Image Endpoint
echo -e "\n\n[2] Testing Process Base64 Image Endpoint..."
# 1x1 Pixel PNG Base64 - Use temp file to avoid "Argument list too long" error
IMAGE_DATA="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
TEMP_PAYLOAD=$(mktemp)
echo "{\"image_base64\": \"$IMAGE_DATA\"}" > "$TEMP_PAYLOAD"
curl -X POST "$BASE_URL/process-base64-image" \
     -H "Content-Type: application/json" \
     -d @"$TEMP_PAYLOAD"
rm -f "$TEMP_PAYLOAD"

# 3. Test File Upload Endpoint
echo -e "\n\n[3] Testing File Upload Endpoint..."
if [ "$KNOWLEDGE_ID" = "REPLACE_WITH_VALID_KNOWLEDGE_ID" ]; then
    echo "  Skipping File Upload: KNOWLEDGE_ID not set in script."
else
    # Ensure test file exists
    if [ ! -f "$TEST_FILE" ]; then
        echo "Creating dummy $TEST_FILE..."
        echo "This is a test file for uploading." > "$TEST_FILE"
    fi

    curl -X POST "$BASE_URL/files" \
         -F "file=@$TEST_FILE" \
         -F "knowledgeID=$KNOWLEDGE_ID"
fi

echo -e "\n\n========================================"
echo "Experiments Completed"
echo "========================================"
