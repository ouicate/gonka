#!/bin/bash

set -euo pipefail

echo "🔐 Getting SSL certificate for proxy..."

if [ -z "${CERT_ISSUER_DOMAIN:-}" ]; then
  echo "❌ CERT_ISSUER_DOMAIN is not set"
  exit 1
fi

mkdir -p /etc/nginx/ssl

# Resolve proxy-ssl host/port (respect KEY_NAME_PREFIX) and node_id
PROXY_SSL_SERVICE_NAME=${PROXY_SSL_SERVICE_NAME:-proxy-ssl}
PROXY_SSL_PORT=${PROXY_SSL_PORT:-8080}
KEY_NAME_PREFIX=${KEY_NAME_PREFIX:-}
FINAL_PROXY_SSL_SERVICE="${KEY_NAME_PREFIX}${PROXY_SSL_SERVICE_NAME}"
PROXY_SSL_BASE_URL="http://${FINAL_PROXY_SSL_SERVICE}:${PROXY_SSL_PORT}"
NODE_ID=${NODE_ID:-proxy}

# Get JWT token
TOKEN_RESPONSE=$(curl -sS -X POST ${PROXY_SSL_BASE_URL}/v1/tokens \
  -H "Content-Type: application/json" \
  -d "{\"node_id\":\"${NODE_ID}\",\"expires_in_days\":30}")

TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.token // empty')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "❌ Failed to obtain JWT token from ${FINAL_PROXY_SSL_SERVICE}:${PROXY_SSL_PORT}"
  echo "$TOKEN_RESPONSE"
  exit 1
fi

# Request certificate bundle
RESPONSE=$(curl -sS -X POST ${PROXY_SSL_BASE_URL}/v1/certs/auto \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"node_id\":\"${NODE_ID}\",\"fqdns\":[\"${CERT_ISSUER_DOMAIN}\"]}")

CERT=$(echo "$RESPONSE" | jq -r '.certificate // empty')
KEY=$(echo "$RESPONSE" | jq -r '.private_key // empty')

if [ -z "$CERT" ] || [ -z "$KEY" ] || [ "$CERT" = "null" ] || [ "$KEY" = "null" ]; then
  echo "❌ Failed to obtain certificate bundle from ${FINAL_PROXY_SSL_SERVICE}:${PROXY_SSL_PORT}"
  echo "$RESPONSE"
  exit 1
fi

echo "$CERT" > /etc/nginx/ssl/cert.pem
echo "$KEY" > /etc/nginx/ssl/private.key

chmod 644 /etc/nginx/ssl/cert.pem
chmod 600 /etc/nginx/ssl/private.key

echo "✅ SSL certificate obtained and installed for ${CERT_ISSUER_DOMAIN}"

# Do not reload nginx here; entrypoint manages configuration and startup

