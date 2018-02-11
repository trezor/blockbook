# initialize from the image

FROM debian:9

RUN apt-get update && apt-get install -y \
    build-essential git wget pkg-config lxc-dev libzmq3-dev libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev

ENV GOLANG_VERSION=go1.9.2.linux-amd64
ENV GOPATH=/go

# install and configure go
RUN cd /opt && wget https://storage.googleapis.com/golang/$GOLANG_VERSION.tar.gz && tar xf $GOLANG_VERSION.tar.gz
RUN ln -s /opt/go/bin/go /usr/bin/go
RUN mkdir -p $GOPATH
RUN echo -n "GO version: " && go version
RUN echo -n "GOPATH: " && echo $GOPATH

# install rocksdb
RUN cd /opt && git clone https://github.com/facebook/rocksdb.git
RUN cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC make static_lib

# install gorocksdb
RUN CGO_CFLAGS="-I/opt/rocksdb/include" CGO_LDFLAGS="-L/opt/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4" go get github.com/tecbot/gorocksdb

RUN go get github.com/golang/glog
RUN go get github.com/graarh/golang-socketio
RUN go get github.com/btcsuite/btcd
RUN go get github.com/gorilla/handlers
RUN go get github.com/bsm/go-vlq
RUN go get github.com/gorilla/handlers
RUN go get github.com/gorilla/mux
RUN go get github.com/pebbe/zmq4
RUN go get github.com/pkg/profile

RUN cd $GOPATH/src && git clone https://github.com/jpochyla/blockbook.git
RUN cd $GOPATH/src/blockbook && go build
