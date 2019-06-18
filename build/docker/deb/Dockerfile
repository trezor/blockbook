# initialize from the image

FROM blockbook-build:latest

ADD gpg-keys /tmp/gpg-keys

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y devscripts debhelper make dh-systemd dh-exec && \
    apt-get clean && \
    gpg --batch --import /tmp/gpg-keys/*

ADD build-deb.sh /build/build-deb.sh

WORKDIR /build
