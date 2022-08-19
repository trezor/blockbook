# initialize from the image defined by BASE_IMAGE
ARG BASE_IMAGE
FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive
ARG PORTABLE_ROCKSDB

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git wget pkg-config lxc-dev libzmq3-dev \
                       libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev \
                       liblz4-dev graphviz && \
    apt-get clean
ARG GOLANG_VERSION
ENV GOLANG_VERSION=go1.17.1
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
ARG TARGETPLATFORM
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ]; then ARCHITECTURE=amd64; elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then ARCHITECTURE=arm64; else ARCHITECTURE=amd64; fi \
    && cd /opt && wget https://dl.google.com/go/$GOLANG_VERSION.linux-$ARCHITECTURE.tar.gz && \
    tar xf $GOLANG_VERSION.linux-$ARCHITECTURE.tar.gz
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
