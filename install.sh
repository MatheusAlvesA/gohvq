#!/bin/bash
set -euo pipefail

# Resolve paths from the script, even when invoked outside the project root.
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd -- "$project_dir"

for command in go install getent id groupadd useradd systemctl mktemp mv; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "Required command not found: $command" >&2
        exit 1
    fi
done

privileged=()
if (( EUID != 0 )); then
    if ! command -v sudo >/dev/null 2>&1; then
        echo "Run as root or install sudo to install the service." >&2
        exit 1
    fi
    privileged=(sudo)
fi

# Build with the invoking user's Go environment before requesting privileges.
bash ./build.sh

if ! getent group gohvq >/dev/null; then
    "${privileged[@]}" groupadd --system gohvq
fi
if ! id -u gohvq >/dev/null 2>&1; then
    "${privileged[@]}" useradd --system --gid gohvq \
        --home-dir /var/lib/gohvq --no-create-home \
        --shell /usr/sbin/nologin gohvq
fi

"${privileged[@]}" install -d -m 0755 /usr/local/bin /etc/systemd/system
"${privileged[@]}" install -d -o gohvq -g gohvq -m 0750 /var/lib/gohvq

# Replace the executable by rename so an already running process is unaffected.
staged_binary="$("${privileged[@]}" mktemp /usr/local/bin/.gohvq.XXXXXXXX)"
trap '"${privileged[@]}" rm -f -- "$staged_binary"' EXIT
"${privileged[@]}" install -o root -g root -m 0755 build/gohvq "$staged_binary"
"${privileged[@]}" mv -f -- "$staged_binary" /usr/local/bin/gohvq

"${privileged[@]}" install -o root -g root -m 0644 \
    gohvq.service /etc/systemd/system/gohvq.service

if "${privileged[@]}" test -e /var/lib/gohvq/gohvq_config.json; then
    echo "Preserving /var/lib/gohvq/gohvq_config.json"
elif [[ -f gohvq_config.json ]]; then
    "${privileged[@]}" install -o root -g gohvq -m 0640 \
        gohvq_config.json /var/lib/gohvq/gohvq_config.json
else
    echo "No configuration copied. Prepare /var/lib/gohvq/gohvq_config.json before starting."
fi

"${privileged[@]}" systemctl daemon-reload
echo "Installed gohvq. The service was not started, restarted, or enabled."
echo "Review /var/lib/gohvq/gohvq_config.json and any TLS paths before starting."
