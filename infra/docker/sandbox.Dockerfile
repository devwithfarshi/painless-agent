# Base sandbox image used by the code_executor tool.
# Each language uses its own upstream image (python:3.12-slim, node:20-alpine, etc.)
# so this file serves as documentation and a custom base for future hardening.
#
# Security constraints are enforced by the Go Docker client at runtime:
#   --network=none  --memory=256m  --cpus=0.5

FROM python:3.12-slim

# Remove package managers that could be used to exfiltrate data.
RUN apt-get purge -y --auto-remove wget curl && rm -rf /var/lib/apt/lists/*

# Drop to a non-root user.
RUN useradd -m -u 1000 sandbox
USER sandbox

WORKDIR /code
