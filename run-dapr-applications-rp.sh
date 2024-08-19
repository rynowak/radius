#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

dapr run \
  --app-id applications-rp \
  --app-port 7010 \
  --dapr-http-port 3503 \
  --dapr-grpc-port 50003