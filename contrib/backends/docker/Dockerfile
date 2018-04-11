FROM debian:9

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && \
    apt-get install -y devscripts debhelper wget tar gzip make dh-systemd && \
    apt-get clean

ADD build.sh /deb/build.sh

WORKDIR /deb

ENTRYPOINT ["/deb/build.sh"]
