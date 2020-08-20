with import <nixpkgs> {};

stdenv.mkDerivation {
  name = "blockbook-dev";
  buildInputs = [
    go
  ];
}
