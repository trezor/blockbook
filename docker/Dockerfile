# initialize from the image

FROM debian:9

RUN apt-get update && apt-get install -y \
    build-essential git wget pkg-config lxc-dev libzmq3-dev libgflags-dev \
    libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev graphviz

ENV GOLANG_VERSION=go1.10.linux-amd64
ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin
ENV CGO_CFLAGS="-I/opt/rocksdb/include"
ENV CGO_LDFLAGS="-L/opt/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4"

# install and configure go
RUN cd /opt && wget https://storage.googleapis.com/golang/$GOLANG_VERSION.tar.gz && \
    tar xf $GOLANG_VERSION.tar.gz
RUN ln -s /opt/go/bin/go /usr/bin/go
RUN mkdir -p $GOPATH
RUN echo -n "GO version: " && go version
RUN echo -n "GOPATH: " && echo $GOPATH

# install rocksdb
RUN cd /opt && git clone https://github.com/facebook/rocksdb.git
RUN cd /opt/rocksdb && CFLAGS=-fPIC CXXFLAGS=-fPIC make static_lib

RUN go get github.com/golang/dep/cmd/dep

# clone repo and ensure dependencies
RUN cd $GOPATH/src && git clone https://github.com/jpochyla/blockbook.git
RUN cd $GOPATH/src/blockbook && dep ensure

# install gorocksdb
RUN cd $GOPATH/src/blockbook/vendor/github.com/tecbot/gorocksdb && \
    go install .

WORKDIR $GOPATH/src/blockbook

CMD go build -o /out/blockbook
