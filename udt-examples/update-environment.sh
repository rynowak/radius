#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

rad resource create 'Applications.Core/environments' default "@$SCRIPT_DIR/environment.json"