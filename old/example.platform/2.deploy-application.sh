#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo "Creating application..."
rad resource create \
    'Applications.Core/applications' \
    my-test-app \
    @application.json

echo "Creating database..."
rad resource create \
    'Example.Platform/globalDatabases' \
    db \
    @database.json

echo "Creating container..."
rad resource create \
    'Applications.Core/containers' \
    webapp \
    @container.json