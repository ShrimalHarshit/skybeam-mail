FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    dovecot-core \
    dovecot-imapd \
    dovecot-lmtpd \
    dovecot-pgsql \
    netcat-openbsd \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Create vmail user/group for Maildir ownership
RUN groupadd -g 5000 vmail && \
    useradd -g vmail -u 5000 vmail -d /var/mail -s /sbin/nologin

# Create mail storage dir with correct permissions
RUN mkdir -p /var/mail/vhosts && \
    chown -R vmail:vmail /var/mail && \
    chmod 770 /var/mail/vhosts

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 143 24

ENTRYPOINT ["/entrypoint.sh"]
