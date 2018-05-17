# initialize from the image

FROM blockbook-build:latest

RUN apt-get update && \
    apt-get install -y devscripts debhelper make dh-systemd dh-exec && \
    apt-get clean

ADD build-deb.sh /build/build-deb.sh
ADD logrotate.sh /build/logrotate.sh

WORKDIR /build

CMD /build/build-deb.sh
