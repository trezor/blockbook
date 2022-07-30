# initialize from the image defined by BASE_IMAGE
FROM ubuntu:18.04
ARG DEBIAN_FRONTEND=noninteractive
ARG PORTABLE_ROCKSDB

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git wget pkg-config lxc-dev libzmq3-dev \
                       libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev \
                       liblz4-dev graphviz && \
    apt-get clean

RUN cd /opt && wget https://download.libsodium.org/libsodium/releases/libsodium-1.0.18-stable.tar.gz && \
    tar zxvf libsodium-1.0.18-stable.tar.gz && cd libsodium-stable && \
    ./configure && make check && make install && ldconfig
RUN cd /opt && wget https://github.com/zeromq/libzmq/releases/download/v4.2.2/zeromq-4.2.2.tar.gz && \
    tar -xvf zeromq-4.2.2.tar.gz && cd zeromq-4.2.2 && ./configure && make && make install

ENV GOLANG_VERSION=go1.17.1.linux-amd64
ENV ROCKSDB_VERSION=v6.22.1
ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin
ENV CGO_CFLAGS="-I/opt/rocksdb/include"
ENV CGO_LDFLAGS="-L/opt/rocksdb -ldl -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4"
ARG TCMALLOC

RUN mkdir /build

RUN if [ -n "${TCMALLOC}" ]; then \
    echo "Using TCMALLOC"; \
    apt-get install -y google-perftools; \
    ln -s /usr/lib/libtcmalloc.so.4 /usr/lib/libtcmalloc.so;\
fi

# install and configure go
RUN cd /opt && wget https://dl.google.com/go/$GOLANG_VERSION.tar.gz && \
    tar xf $GOLANG_VERSION.tar.gz
RUN ln -s /opt/go/bin/go /usr/bin/go
RUN mkdir -p $GOPATH
RUN echo -n "GO version: " && go version
RUN echo -n "GOPATH: " && echo $GOPATH

# install rocksdb
RUN cd /opt && git clone -b $ROCKSDB_VERSION --depth 1 https://github.com/facebook/rocksdb.git
RUN cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC PORTABLE=$PORTABLE_ROCKSDB make -j 4 release
RUN strip /opt/rocksdb/ldb /opt/rocksdb/sst_dump && \
    cp /opt/rocksdb/ldb /opt/rocksdb/sst_dump /build

# pre-load depencencies
RUN \
    cleanup() { rm -rf $GOPATH/src/github.com/trezor ; } && \
    trap cleanup EXIT && \
    mkdir -p $GOPATH/src/github.com/trezor && \
    cd $GOPATH/src/github.com/trezor && \
    git clone https://github.com/trezor/blockbook.git && \
    cd blockbook && \
    go mod download

ADD Makefile /build/Makefile

VOLUME /out

WORKDIR /build
