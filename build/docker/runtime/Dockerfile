# Runtime image: the binary + static assets on top of the build image.
# Config is passed at run time via -blockchaincfg, so one image serves any network.

FROM blockbook-build:latest

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# blockbook resolves ./static and ./openapi.yaml relative to the working directory.
WORKDIR /src

COPY static /src/static
COPY openapi.yaml /src/openapi.yaml
COPY blockbook /src/blockbook

RUN chmod +x /src/blockbook

ENTRYPOINT ["/src/blockbook"]
