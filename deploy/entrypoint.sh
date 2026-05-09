#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Container entrypoint for the Luna peer image. Generates a self-signed
# TLS certificate at startup if one is not already present, then execs
# the requested binary. Operators running in production should replace
# the generated cert with one issued by their chosen CA.

set -e

CERT_DIR="${LUNA_CERT_DIR:-/home/luna/.grap3}"
CERT_KEY="${CERT_DIR}/luna-tls.key"
CERT_CRT="${CERT_DIR}/luna-tls.crt"

if [ ! -f "${CERT_KEY}" ] || [ ! -f "${CERT_CRT}" ]; then
    echo "[entrypoint] generating self-signed TLS cert at ${CERT_DIR}"
    openssl req -x509 -nodes -newkey rsa:2048 \
        -keyout "${CERT_KEY}" \
        -out "${CERT_CRT}" \
        -days 365 \
        -subj "/CN=${LUNA_TLS_CN:-localhost}" \
        >/dev/null 2>&1
fi

exec "$@"
