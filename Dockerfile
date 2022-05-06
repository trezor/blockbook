FROM golang:1.14.2-buster AS build

RUN apt-get update && apt-get install -y \
    build-essential git wget pkg-config libzmq3-dev \
    libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev libtool \
    && rm -rf /var/lib/apt/lists/* \
    && git clone https://github.com/facebook/rocksdb.git \
    && cd rocksdb \
    && git checkout v6.8.1 \
    && CFLAGS=-fPIC CXXFLAGS=-fPIC make release

ENV CGO_CFLAGS="-I/go/rocksdb/include" \
    CGO_LDFLAGS="-L/go/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4"

RUN apt-get install libzmq3-dev \
    && cd libzmq \
    && ./autogen.sh \
    && ./configure \
    && make \
    && make install

COPY . ./blockbook
WORKDIR /go/blockbook

RUN go build

FROM debian:buster

RUN apt-get update && apt-get install -y \
    build-essential git wget pkg-config libzmq3-dev \
    libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev libtool \
    && rm -rf /var/lib/apt/lists/* \
    && git clone https://github.com/facebook/rocksdb.git \
    && cd rocksdb \
    && git checkout v6.8.1 \
    && CFLAGS=-fPIC CXXFLAGS=-fPIC make release

ENV CGO_CFLAGS="-I/go/rocksdb/include" \
    CGO_LDFLAGS="-L/go/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4"

RUN apt-get install libzmq3-dev \
    && cd libzmq \
    && ./autogen.sh \
    && ./configure \
    && make \
    && make install

COPY --from=build /go/blockbook /go/blockbook
WORKDIR /blockbook
RUN mkdir /etc/blockbook
