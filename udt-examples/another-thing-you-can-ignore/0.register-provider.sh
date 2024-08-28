#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo "Registering Comcast.Platform resource provider..."
rad resourceprovider create 'Comcast.Platform' @Comcast.Platform.json