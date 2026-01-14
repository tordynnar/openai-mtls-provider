#!/bin/bash
#
# Generate certificates for mTLS authentication
# Usage: ./generate.sh
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Generating mTLS certificates..."
echo ""

# Configuration
DAYS=365
KEY_SIZE=4096

# CA details
CA_SUBJ="/C=US/ST=Test/L=Test/O=MockOpenAI/CN=MockOpenAI-CA"

# Server details
SERVER_SUBJ="/C=US/ST=Test/L=Test/O=MockOpenAI/CN=localhost"

# Client details
CLIENT_SUBJ="/C=US/ST=Test/L=Test/O=MockOpenAI/CN=test-client"

# Clean up old certificates
echo "Cleaning up old certificates..."
rm -f ca.key ca.crt ca.srl
rm -f server.key server.csr server.crt server.ext
rm -f client.key client.csr client.crt client.ext

# Generate CA
echo "Generating CA certificate..."
openssl genrsa -out ca.key $KEY_SIZE 2>/dev/null
openssl req -new -x509 -days $DAYS -key ca.key -out ca.crt -subj "$CA_SUBJ"
echo "  Created: ca.key, ca.crt"

# Generate Server certificate
echo "Generating server certificate..."
openssl genrsa -out server.key $KEY_SIZE 2>/dev/null
openssl req -new -key server.key -out server.csr -subj "$SERVER_SUBJ"

cat > server.ext << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days $DAYS -extfile server.ext 2>/dev/null
echo "  Created: server.key, server.crt"

# Generate Client certificate
echo "Generating client certificate..."
openssl genrsa -out client.key $KEY_SIZE 2>/dev/null
openssl req -new -key client.key -out client.csr -subj "$CLIENT_SUBJ"

cat > client.ext << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
extendedKeyUsage = clientAuth
EOF

openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out client.crt -days $DAYS -extfile client.ext 2>/dev/null
echo "  Created: client.key, client.crt"

# Clean up CSR and extension files
rm -f server.csr server.ext client.csr client.ext

echo ""
echo "Certificate generation complete!"
echo ""
echo "Files created:"
echo "  CA:     ca.crt, ca.key"
echo "  Server: server.crt, server.key"
echo "  Client: client.crt, client.key"
echo ""
echo "Usage:"
echo "  Server: ./openai-mock-server -cert ../certs/server.crt -key ../certs/server.key -ca ../certs/ca.crt"
echo "  Client: ./openai-test-client -cert ../certs/client.crt -key ../certs/client.key -ca ../certs/ca.crt"
