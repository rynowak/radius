#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

echo "Registering Example.Platform resource provider..."
rad resourceprovider create 'Example.Platform' @Example.Platform.json