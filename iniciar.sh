#!/bin/sh
set -eu
cd -- "$(dirname -- "$0")"
exec ./reembolso -addr 0.0.0.0:5002 "$@"
