# initialize from the image defined by BASE_IMAGE
ARG BASE_IMAGE
FROM $BASE_IMAGE
ARG DEBIAN_FRONTEND=noninteractive
ARG PORTABLE_ROCKSDB

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git wget pkg-config lxc-dev libzmq3-dev \
                       libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev \
                       libzstd-dev liblz4-dev graphviz \
                       autoconf automake libtool && \
    apt-get clean
ARG GOLANG_VERSION
ENV GOLANG_VERSION=go1.25.4
ENV ROCKSDB_VERSION=v11.1.1
ENV ROCKSDB_COMMIT=6cdeb9d9d0630763327f512e6255cab33f6834e7
ENV LIBZMQ_VERSION=v4.3.5
ENV LIBZMQ_COMMIT=622fc6dde99ee172ebaa9c8628d85a7a1995a21d
ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin
# Static linkage: blockbook is linked against librocksdb.a + a static libzmq.a +
# the compression libs, then libstdc++/libgcc are pulled in statically. Only glibc
# and a few system libs remain dynamic. This removes the runtime dependency on a
# shared librocksdb / libzmq, so the RocksDB ABI/version can never mismatch at runtime.
#   -Wl,-Bstatic ... -Wl,-Bdynamic brackets the libraries we want archived in;
#   everything after -Bdynamic (pthread, dl, m, glibc) links dynamically as usual.
ENV CGO_CFLAGS="-I/opt/rocksdb/include -I/opt/libzmq/include -DZMQ_STATIC"
ENV CGO_LDFLAGS="-L/opt/rocksdb -L/opt/libzmq/src/.libs \
-Wl,-Bstatic -lrocksdb -lzmq -llz4 -lzstd -lz -lsnappy -lbz2 -lstdc++ \
-Wl,-Bdynamic -lm -lpthread -ldl \
-static-libgcc -static-libstdc++"
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
    && test "$(git -C /opt/rocksdb rev-parse HEAD)" = $ROCKSDB_COMMIT
RUN cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC PORTABLE=$PORTABLE_ROCKSDB DISABLE_WARNING_AS_ERROR=1 make -j 4 release
RUN strip /opt/rocksdb/ldb /opt/rocksdb/sst_dump && \
    cp /opt/rocksdb/ldb /opt/rocksdb/sst_dump /build

# install libzmq (static)
# The libzmq3-dev package only ships a shared library; to link zmq statically we build
# libzmq from source and keep the static archive at /opt/libzmq/src/.libs/libzmq.a.
# --without-libsodium keeps the dependency set minimal (blockbook uses plain tcp://).
RUN cd /opt && git clone -b $LIBZMQ_VERSION --depth 1 https://github.com/zeromq/libzmq.git \
    && test "$(git -C /opt/libzmq rev-parse HEAD)" = $LIBZMQ_COMMIT
RUN cd /opt/libzmq && ./autogen.sh && \
    ./configure --enable-static --disable-shared --without-libsodium \
                --disable-Werror CFLAGS=-fPIC CXXFLAGS=-fPIC && \
    make -j 4

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
