# initialize from the image

FROM debian:9

ENV DEBIAN_FRONTEND="noninteractive"
ENV GOLANG_VERSION="go1.12.4.linux-amd64"
ENV ROCKSDB_VERSION="v5.18.3"
ENV GOPATH="/go"
ENV PATH="$PATH:$GOPATH/bin"
ENV CGO_CFLAGS="-I/opt/rocksdb/include"
ENV CGO_LDFLAGS="-L/opt/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4"

# upgrade the system and install dependencies
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y build-essential git wget pkg-config lxc-dev libzmq3-dev \
                       libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev \
                       liblz4-dev graphviz && \
    apt-get clean && \
    mkdir /build && \
\
`# install and configure go` \
    cd /opt && \
    wget -q --progress=bar "https://storage.googleapis.com/golang/$GOLANG_VERSION.tar.gz" && \
    tar xf $GOLANG_VERSION.tar.gz && \
    ln -s /opt/go/bin/go /usr/bin/go && \
    mkdir -p "$GOPATH" && \
    echo -n "GO version: " && go version && \
    echo -n "GOPATH: " && echo "$GOPATH" && \
\
`# install rocksdb` \
    cd /opt && \
    git clone -b $ROCKSDB_VERSION --depth 1 https://github.com/facebook/rocksdb.git && \
    cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC make -j 4 release && \
    strip /opt/rocksdb/ldb /opt/rocksdb/sst_dump && \
    cp /opt/rocksdb/ldb /opt/rocksdb/sst_dump /build && \
\
`# install build tools` \
    go get github.com/golang/dep/cmd/dep && \
    go get github.com/gobuffalo/packr/... && \
\
`# download pre-loaded depencencies` \
    trap 'rm -rf "$GOPATH/src/blockbook";' EXIT && \
    cd "$GOPATH/src" && \
    git clone https://github.com/trezor/blockbook.git && \
    cd blockbook && \
    dep ensure -vendor-only && \
    cp -r vendor /build/vendor

ADD Makefile /build/Makefile

VOLUME /out

WORKDIR /build
