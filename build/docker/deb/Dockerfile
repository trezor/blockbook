# initialize from the image

FROM blockbook-build:latest

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y devscripts debhelper make dh-exec zstd && \
    apt-get clean

# install docker cli
ARG DOCKER_VERSION
ARG DOCKER_SHA256

RUN if [ -z "$DOCKER_VERSION" ]; then echo "DOCKER_VERSION is a required build arg" && exit 1; fi
RUN if [ -z "$DOCKER_SHA256" ]; then echo "DOCKER_SHA256 is a required build arg" && exit 1; fi

RUN wget -O docker.tgz "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz" && \
    echo "${DOCKER_SHA256}  docker.tgz" | sha256sum -c - && \
    tar -xzf docker.tgz --strip 1 -C /usr/local/bin/ && \
    rm docker.tgz && \
    docker --version

ADD gpg-keys /tmp/gpg-keys
RUN gpg --batch --import /tmp/gpg-keys/*

ADD build-deb.sh /build/build-deb.sh

WORKDIR /build
