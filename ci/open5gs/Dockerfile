# The Open5GS HSS (open5gs-hssd) as an independent S6a and Cx peer for the
# capture job (.github/workflows/peer.yml). Built from the upstream source
# at a pinned release; it runs in CI only and nothing here is used by the
# library.
FROM ubuntu:noble-20261024.1@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852

# Build dependencies from the Open5GS guide "Building Open5GS from Sources".
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates git openssl meson ninja-build build-essential flex bison cmake pkg-config \
      python3-pip python3-setuptools python3-wheel \
      libsctp-dev libgnutls28-dev libgcrypt-dev libssl-dev libmongoc-dev libbson-dev libyaml-dev \
      libnghttp2-dev libmicrohttpd-dev libcurl4-gnutls-dev libtins-dev libtalloc-dev libidn-dev \
 && rm -rf /var/lib/apt/lists/*

ARG OPEN5GS_TAG=v2.8.0
ARG OPEN5GS_COMMIT=157f611a530e292e40ec50f9d23f0ef5d4fcd6a6
RUN git clone --depth 1 --branch "$OPEN5GS_TAG" https://github.com/open5gs/open5gs /src \
 && test "$(git -C /src rev-parse HEAD)" = "$OPEN5GS_COMMIT" \
 && cd /src \
 && meson setup build --prefix=/open5gs --libdir=lib \
 && ninja -C build install \
 && find /open5gs -name 'dict_*.fdx' -o -name 'dbg_msg_dumps.fdx' -o -name open5gs-hssd \
 && rm -rf /src

# The prometheus-client-c subproject installs under the multiarch libdir.
ENV LD_LIBRARY_PATH=/open5gs/lib:/open5gs/lib/x86_64-linux-gnu

# freeDiameter refuses to start without TLS credentials, even when every
# connection is plain TCP. A throwaway self-signed certificate is enough.
RUN mkdir -p /etc/open5gs/tls \
 && openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
      -subj "/CN=hss.test" \
      -keyout /etc/open5gs/tls/hss.key -out /etc/open5gs/tls/hss.crt

COPY hss.yaml hss.conf /etc/open5gs/

EXPOSE 3868
CMD ["/open5gs/bin/open5gs-hssd", "-c", "/etc/open5gs/hss.yaml"]
