with import <nixpkgs> {};

stdenv.mkDerivation {
  name = "blockbook-dev";
  buildInputs = [
    bzip2
    go
    lz4
    pkg-config
    rocksdb
    snappy
    zeromq
    zlib
    gcc
  ];
  shellHook = ''
    export CGO_LDFLAGS="-L${stdenv.cc.cc.lib}/lib -lrocksdb -lz -lbz2 -lsnappy -llz4 -lm -lstdc++"
  '';
}
