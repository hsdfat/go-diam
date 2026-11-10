# freeDiameter as an independent peer for the capture job (.github/workflows/peer.yml).
# It runs in CI only; nothing here is used by the library.
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends freediameter freediameter-extensions openssl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# freeDiameter refuses to start without TLS credentials, even when every
# connection is plain TCP. A throwaway self-signed certificate is enough.
RUN mkdir -p /etc/freeDiameter \
 && openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
      -subj "/CN=peer.test" \
      -keyout /etc/freeDiameter/key.pem -out /etc/freeDiameter/cert.pem \
 && openssl dhparam -out /etc/freeDiameter/dh.pem 2048

COPY freeDiameter.conf acl_wl.conf /etc/freeDiameter/

EXPOSE 3868
CMD ["freeDiameterd", "-c", "/etc/freeDiameter/freeDiameter.conf", "-d"]
