#!/bin/bash

# Simple script to open Slack's channel creation dialog
# You just need to click "Create" button

CHANNEL_NAME="${1:-test-channel}"
WORKSPACE_ID="E08K7E7N092"  # Your Enterprise ID

# Open Slack in browser with channel creation intent
# This should open the "Create Channel" dialog
open "https://app.slack.com/client/${WORKSPACE_ID}/?channel_create=1&channel_name=${CHANNEL_NAME}"

echo "Browser opened. Please click 'Create' to create channel: ${CHANNEL_NAME}"