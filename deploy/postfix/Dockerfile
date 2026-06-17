FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    postfix \
    postfix-pcre \
    libsasl2-modules \
    sasl2-bin \
    ca-certificates \
    wget \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Copy entrypoint script that runs postmap before starting postfix
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Config files are bind-mounted at runtime (see docker-compose.yml)
# Cert files expected at /etc/postfix/certs/ (bind mount from host)

EXPOSE 25 587

ENTRYPOINT ["/entrypoint.sh"]
