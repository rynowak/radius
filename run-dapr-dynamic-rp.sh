#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

dapr run \
  --app-id dynamic-rp \
  --app-port 7011 \
  --dapr-http-port 3504 \
  --dapr-grpc-port 50004