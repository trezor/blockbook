FROM debian:9

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && \
    apt-get install -y devscripts debhelper wget tar gzip make dh-systemd && \
    apt-get clean

ADD build.sh /deb/build.sh

ADD gpg-keys /tmp/gpg-keys
RUN gpg --import /tmp/gpg-keys/*

WORKDIR /deb

ENTRYPOINT ["/deb/build.sh"]
