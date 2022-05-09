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

RUN git clone https://github.com/zeromq/libzmq \
    && cd libzmq \
    && ./autogen.sh \
    && ./configure \
    && make \
    && make install

COPY . ./blockbook

RUN cd blockbook \
    && go build

FROM debian:buster

COPY --from=build /go/rocksdb /go

ENV CGO_CFLAGS="-I/go/rocksdb/include" \
    CGO_LDFLAGS="-L/go/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4"

RUN apt-get update && apt-get install -y \
    libsnappy-dev build-essential libzmq3-dev \
    libtool pkg-config

COPY --from=build /go/libzmq /go

COPY --from=build /go/blockbook /go/blockbook
WORKDIR /go/blockbook
RUN mkdir /etc/blockbook
