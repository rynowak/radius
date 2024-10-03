#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

dapr run \
  --app-id ucp \
  --app-port 9000 \
  --dapr-http-port 3502 \
  --dapr-grpc-port 50002 \
  --resources-path ./components