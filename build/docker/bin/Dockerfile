# initialize from the image defined by BASE_IMAGE
ARG BASE_IMAGE
FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive
ARG PORTABLE_ROCKSDB

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git wget pkg-config lxc-dev libzmq3-dev \
                       libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev \
                       libzstd-dev liblz4-dev graphviz && \
    apt-get clean
ARG GOLANG_VERSION
ENV GOLANG_VERSION=go1.25.4
ENV ROCKSDB_VERSION=v9.10.0
ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin
ENV CGO_CFLAGS="-I/opt/rocksdb/include"
ENV CGO_LDFLAGS="-L/opt/rocksdb -ldl -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4 -lzstd"
ARG TCMALLOC

RUN mkdir /build

RUN if [ -n "${TCMALLOC}" ]; then \
    echo "Using TCMALLOC"; \
    apt-get install -y google-perftools; \
    ln -s /usr/lib/libtcmalloc.so.4 /usr/lib/libtcmalloc.so;\
fi

# install and configure go
ARG TARGETPLATFORM
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ]; then ARCHITECTURE=amd64; GO_SHA256=9fa5ffeda4170de60f67f3aa0f824e426421ba724c21e133c1e35d6159ca1bec; \
    elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then ARCHITECTURE=arm64; GO_SHA256=a68e86d4b72c2c2fecf7dfed667680b6c2a071221bbdb6913cf83ce3f80d9ff0; \
    elif [ "$TARGETPLATFORM" = "linux/aarch64" ]; then ARCHITECTURE=arm64; GO_SHA256=a68e86d4b72c2c2fecf7dfed667680b6c2a071221bbdb6913cf83ce3f80d9ff0; \
    else ARCHITECTURE=amd64; GO_SHA256=9fa5ffeda4170de60f67f3aa0f824e426421ba724c21e133c1e35d6159ca1bec; fi \
    && cd /opt && wget https://dl.google.com/go/$GOLANG_VERSION.linux-$ARCHITECTURE.tar.gz \
    && echo "$GO_SHA256  $GOLANG_VERSION.linux-$ARCHITECTURE.tar.gz" | sha256sum -c - \
    && tar xf $GOLANG_VERSION.linux-$ARCHITECTURE.tar.gz
RUN ln -s /opt/go/bin/go /usr/bin/go
RUN mkdir -p $GOPATH
RUN echo -n "GO version: " && go version
RUN echo -n "GOPATH: " && echo $GOPATH

# install rocksdb
RUN cd /opt && git clone -b $ROCKSDB_VERSION --depth 1 https://github.com/facebook/rocksdb.git \
    && test "$(git -C /opt/rocksdb rev-parse HEAD)" = "ae8fb3e5000e46d8d4c9dbf3a36019c0aaceebff"
RUN cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC PORTABLE=$PORTABLE_ROCKSDB DISABLE_WARNING_AS_ERROR=1 make -j 4 release
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
