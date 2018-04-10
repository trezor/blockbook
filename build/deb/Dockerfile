# initialize from the image

FROM blockbook-build:latest

RUN apt-get update && apt-get install -y \
    devscripts debhelper make dh-systemd dh-exec

ADD build-deb.sh /build/build-deb.sh

WORKDIR /build

CMD /build/build-deb.sh
