#!/usr/bin/env bash

set -euo pipefail

PG_USER=admin
PG_DATABASE=kubevirt-provider
PG_HOST=127.0.0.1
PG_PORT=5432
export PGPASSWORD=adminpass

until podman exec kubevirt-provider-db pg_isready -U ${PG_USER} --dbname ${PG_DATABASE} --host ${PG_HOST} --port ${PG_PORT}; do sleep 1; done
