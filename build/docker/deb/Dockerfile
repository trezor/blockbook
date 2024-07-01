# initialize from the image

FROM blockbook-build:latest

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y devscripts debhelper make dh-exec zstd && \
    apt-get clean

ADD gpg-keys /tmp/gpg-keys
RUN gpg --batch --import /tmp/gpg-keys/*

ADD build-deb.sh /build/build-deb.sh

WORKDIR /build
