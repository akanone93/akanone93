#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Generates the Compose stack's credentials into .secrets/ on first run,
# its development TLS certificate (dev_tls below), the mutual-TLS identities
# of the roles (internal_tls) and the function-artifact key (function_key).
# Nothing here is ever committed (.gitignore), and nothing has a default:
# every password is 32 random bytes from the operating system.
set -eu
cd "$(dirname "$0")"
umask 077
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

credentials() {
  mkdir .secrets
  rand() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
  admin=$(rand) pg=$(rand) s3key=$(rand | cut -c1-20) s3secret=$(rand) grafana=$(rand) keycloak=$(rand) analytics=$(rand)
  # The image's own user is the administrator; plux-server connects as the
  # role postgres-init.sh creates, which row-level security binds.
  printf 'POSTGRES_USER=postgres\nPOSTGRES_DB=plux\nPOSTGRES_PASSWORD=%s\nPLUX_APP_PASSWORD=%s\nPLUX_ANALYTICS_READER_PASSWORD=%s\n' "$admin" "$pg" "$analytics" > .secrets/postgres.env
  printf 'PLUX_DATABASE_URL=postgres://plux:%s@postgres:5432/plux?sslmode=disable\nPLUX_OBJECT_STORAGE_ACCESS_KEY_ID=%s\nPLUX_OBJECT_STORAGE_SECRET_ACCESS_KEY=%s\n' \
    "$pg" "$s3key" "$s3secret" > .secrets/plux-server.env
  printf '{"identities":[{"name":"plux","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]}]}\n' \
    "$s3key" "$s3secret" > .secrets/s3.json
  printf 'GF_SECURITY_ADMIN_USER=admin\nGF_SECURITY_ADMIN_PASSWORD=%s\nPLUX_ANALYTICS_READER_PASSWORD=%s\n' "$grafana" "$analytics" > .secrets/grafana.env
  printf 'KC_BOOTSTRAP_ADMIN_USERNAME=admin\nKC_BOOTSTRAP_ADMIN_PASSWORD=%s\n' "$keycloak" > .secrets/keycloak.env
  # The S3 config is read by the SeaweedFS container's user.
  chmod 644 .secrets/s3.json
  echo "Generated credentials in $(pwd)/.secrets (Grafana admin password in grafana.env)."
}

# The API serves TLS 1.3 here as it does in production (SEC-040). The
# certificate is development-only: a local CA (1 year, constrained to
# localhost, the service name and the Android emulator's host address, so
# trusting it on a machine cannot vouch for any other site) signs a server
# certificate (90 days) whose key is kept across renewals, so a pin of it
# stays valid. The CA key stays in .secrets/ca and is never mounted into a
# container; .secrets/tls holds what the server and Prometheus read. A
# production installation uses its own certificate, so this refuses to run
# for a configuration that declares itself production.
dev_tls() {
  if grep -Eq '^[[:space:]]*production:[[:space:]]*true' plux-server.yaml; then
    echo "init-secrets.sh: plux-server.yaml sets production: true; the self-signed development certificate is refused. Configure server.tls with your own certificate." >&2
    exit 1
  fi
  command -v openssl >/dev/null 2>&1 || { echo "init-secrets.sh: openssl is required to create the development TLS certificate" >&2; exit 1; }
  mkdir -p .secrets/ca .secrets/tls
  month=2592000
  renew=0
  if [ ! -s .secrets/ca/ca.crt ] || ! openssl x509 -checkend "$month" -noout -in .secrets/ca/ca.crt >/dev/null; then
    cat > "$tmp/ca.cnf" <<'CNF'
[req]
distinguished_name = dn
prompt = no
x509_extensions = ext
[dn]
O = Plux development
CN = Plux development CA
[ext]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
nameConstraints = critical,permitted;DNS:localhost,permitted;DNS:plux-server,permitted;IP:127.0.0.1/255.255.255.255,permitted;IP:10.0.2.2/255.255.255.255,permitted;IP:::1/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
CNF
    openssl ecparam -name prime256v1 -genkey -noout -out .secrets/ca/ca.key
    openssl req -new -x509 -sha256 -days 365 -key .secrets/ca/ca.key -config "$tmp/ca.cnf" -out .secrets/ca/ca.crt
    renew=1
    echo "Generated the development CA .secrets/ca/ca.crt (valid 1 year): trust it as described in README.md."
  fi
  if [ "$renew" = 0 ] && [ -s .secrets/tls/server.crt ] && openssl x509 -checkend "$month" -noout -in .secrets/tls/server.crt >/dev/null; then
    return 0
  fi
  cat > "$tmp/leaf.cnf" <<'CNF'
[req]
distinguished_name = dn
prompt = no
[dn]
CN = localhost
[ext]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
subjectAltName = @san
[san]
DNS.1 = localhost
DNS.2 = plux-server
IP.1 = 127.0.0.1
IP.2 = ::1
IP.3 = 10.0.2.2
CNF
  [ -s .secrets/tls/server.key ] || openssl ecparam -name prime256v1 -genkey -noout -out .secrets/tls/server.key
  openssl req -new -key .secrets/tls/server.key -config "$tmp/leaf.cnf" -out "$tmp/leaf.csr"
  openssl x509 -req -sha256 -days 90 -in "$tmp/leaf.csr" -CA .secrets/ca/ca.crt -CAkey .secrets/ca/ca.key \
    -CAcreateserial -extfile "$tmp/leaf.cnf" -extensions ext -out .secrets/tls/server.crt 2>/dev/null
  cp .secrets/ca/ca.crt .secrets/tls/ca.crt
  # The server runs as another user (65532) than the one who ran this
  # script, so, like s3.json, its files must be world-readable; the
  # directory above them (.secrets, mode 0700) keeps other users out.
  chmod 755 .secrets/tls
  chmod 644 .secrets/tls/server.key .secrets/tls/server.crt .secrets/tls/ca.crt
  echo "Issued the development server certificate .secrets/tls/server.crt (valid 90 days; restart plux-server to load a renewed one)."
}

# The roles api/worker (plux-server), fnrunner, egress and builder talk over
# mutual TLS 1.3 (SEC-043): each presents a certificate signed by one
# internal CA and trusts no other. The CA is separate from the development
# CA above, which people install in their operating system: trusting that
# one must never vouch for a role. The internal CA (1 year) is name-
# constrained to the four service names, so even its key could not vouch for
# any other host; its key stays in .secrets/ca and is mounted nowhere. Each
# role gets a directory with only its own certificate (90 days, serverAuth
# and clientAuth, because every role is both) and key, and the CA certificate,
# so a role cannot read another role's key. Keys are kept across renewals.
internal_tls() {
  command -v openssl >/dev/null 2>&1 || { echo "init-secrets.sh: openssl is required to create the internal TLS identities" >&2; exit 1; }
  mkdir -p .secrets/ca .secrets/internal
  month=2592000
  renew=0
  if [ ! -s .secrets/ca/internal-ca.crt ] || ! openssl x509 -checkend "$month" -noout -in .secrets/ca/internal-ca.crt >/dev/null; then
    cat > "$tmp/internal-ca.cnf" <<'CNF'
[req]
distinguished_name = dn
prompt = no
x509_extensions = ext
[dn]
O = Plux development
CN = Plux internal CA
[ext]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
nameConstraints = critical,permitted;DNS:plux-server,permitted;DNS:plux-fnrunner,permitted;DNS:plux-egress,permitted;DNS:plux-builder
CNF
    openssl ecparam -name prime256v1 -genkey -noout -out .secrets/ca/internal-ca.key
    openssl req -new -x509 -sha256 -days 365 -key .secrets/ca/internal-ca.key -config "$tmp/internal-ca.cnf" -out .secrets/ca/internal-ca.crt
    renew=1
    echo "Generated the internal CA .secrets/ca/internal-ca.crt (valid 1 year); it is not for installing anywhere."
  fi
  for svc in plux-server plux-fnrunner plux-egress plux-builder; do
    dir=.secrets/internal/$svc
    mkdir -p "$dir"
    # The roles run as another user than the one running this script; the
    # directory above (.secrets, mode 0700) keeps other users out.
    chmod 755 .secrets/internal "$dir"
    if [ "$renew" = 0 ] && [ -s "$dir/tls.crt" ] && openssl x509 -checkend "$month" -noout -in "$dir/tls.crt" >/dev/null; then
      continue
    fi
    cat > "$tmp/$svc.cnf" <<CNF
[req]
distinguished_name = dn
prompt = no
[dn]
CN = $svc
[ext]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = serverAuth,clientAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
subjectAltName = DNS:$svc
CNF
    [ -s "$dir/tls.key" ] || openssl ecparam -name prime256v1 -genkey -noout -out "$dir/tls.key"
    openssl req -new -key "$dir/tls.key" -config "$tmp/$svc.cnf" -out "$tmp/$svc.csr"
    openssl x509 -req -sha256 -days 90 -in "$tmp/$svc.csr" -CA .secrets/ca/internal-ca.crt -CAkey .secrets/ca/internal-ca.key \
      -CAcreateserial -extfile "$tmp/$svc.cnf" -extensions ext -out "$dir/tls.crt" 2>/dev/null
    cp .secrets/ca/internal-ca.crt "$dir/ca.crt"
    chmod 644 "$dir/tls.key" "$dir/tls.crt" "$dir/ca.crt"
    echo "Issued the internal certificate $dir/tls.crt (valid 90 days; restart $svc to load a renewed one)."
  done
}

# The key that signs function artifacts (FN-009, ADR-0063). The file signing
# backend would generate it inside the plux-keys volume on first use, but
# fnrunner takes the public keys it trusts from its own configuration only
# and never from the api role, so the key is made here, where both halves are
# known: plux-server mounts the private key as the backend's file
# function-artifact.ed25519 (64 bytes: seed, then public key), and fnrunner's
# configuration, plux-fnrunner.yaml in .secrets/, lists the public key under
# the identifier the backend gives it. The key is for this development stack
# only: the file backend never signs for a production environment (SEC-056).
function_key() {
  command -v openssl >/dev/null 2>&1 || { echo "init-secrets.sh: openssl is required to create the function-artifact key" >&2; exit 1; }
  key=.secrets/function-artifact.ed25519
  if [ ! -s "$key" ]; then
    openssl genpkey -algorithm ed25519 -outform DER -out "$tmp/function.der"
    openssl pkey -inform DER -in "$tmp/function.der" -pubout -outform DER -out "$tmp/function.pub.der"
    # PKCS#8 and SubjectPublicKeyInfo end in the raw 32-byte seed and public key.
    { tail -c 32 "$tmp/function.der"; tail -c 32 "$tmp/function.pub.der"; } > "$key"
    chmod 644 "$key"
    echo "Generated the function-artifact key $key."
  fi
  # The identifier is the first 16 bytes of the public key's SHA-256, in
  # hexadecimal (signing.KeyID).
  keyid=$(tail -c 32 "$key" | openssl dgst -sha256 -binary | head -c 16 | od -An -tx1 | tr -d ' \n')
  pub=$(tail -c 32 "$key" | openssl base64 -A)
  # plux-fnrunner.yaml ends with the fnrunner section, which this continues.
  { cat plux-fnrunner.yaml; printf '  artifactKeys:\n    - keyID: "%s"\n      publicKey: "%s"\n' "$keyid" "$pub"; } > .secrets/plux-fnrunner.yaml
  chmod 644 .secrets/plux-fnrunner.yaml
}

[ -d .secrets ] || credentials
dev_tls
internal_tls
function_key
