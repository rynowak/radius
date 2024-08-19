#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

mkdir -p "$SCRIPT_DIR/tmp"
npm --prefix "$SCRIPT_DIR/../hack/bicep-types-radius/src/manifest-to-bicep" run build
npm --prefix "$SCRIPT_DIR/../hack/bicep-types-radius/src/manifest-to-bicep" run start \
  generate \
  "$SCRIPT_DIR/Example.Platform.yaml" \
  "$SCRIPT_DIR/tmp"

bicep publish-extension \
    "$SCRIPT_DIR/tmp/index.json" \
    --target "$SCRIPT_DIR/Example.Platform.tgz"