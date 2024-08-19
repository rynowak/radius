#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# check if first argument is provided
if [ -z "$1" ]
then
  echo "Usage: $0 <new-password>"
  exit 1
fi

kubectl annotate deployment postgres -n default-udt-demo --overwrite password="$1" 