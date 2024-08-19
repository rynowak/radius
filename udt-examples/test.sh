#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo "Registering Resource Provider..."
rad resourceprovider create "$SCRIPT_DIR/Example.Platform.yaml"
echo ""

echo "Creating Environment..."
rad resource create 'Applications.Core/environments' default @"$SCRIPT_DIR/environment.json"
echo ""

echo "Deploying app.bicep..."
rad deploy "$SCRIPT_DIR/app.bicep" -a radius-demo

# echo "Creating Application..."
# rad resource create 'Applications.Core/applications' radius-demo @"$SCRIPT_DIR/application.json"

# echo "Creating OpenAI resource..."
# rad resource create 'Example.Platform/openAIServices' ai @"$SCRIPT_DIR/ai.json"