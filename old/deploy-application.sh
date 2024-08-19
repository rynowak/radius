#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

rad resource create 'Applications.Core/applications' udt-demo "@$SCRIPT_DIR/application.json"
rad resource create 'Contoso.Example/postgreSQLDatabases' db  "@$SCRIPT_DIR/postgreSQLDatabases.json" -o json
rad run "$SCRIPT_DIR/app.bicep"