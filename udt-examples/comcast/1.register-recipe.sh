#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo "Registering Comcast.Platform/globalDatabases recipe..."
rad recipe register \
    default \
    --resource-type Comcast.Platform/globalDatabases \
    --template-kind terraform \
    --template-path git::github.com/rynowak/private-recipes \
    --parameters size=small