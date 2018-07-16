# CashAddr Converter

This repository is a go implementation of the cashaddr and copay Bitcoin Cash
address formats.  The intent is to provide a command line tool, a
reference implementation, and a microservice for use by the Bitcoin Cash
community.

# Prerequisites

The following are only required for `make image` and `make run`.

Install docker: https://docs.docker.com/engine/installation/linux/docker-ce/

Add your user to the docker group: `sudo usermod -aG docker ${USER}` and reload the group: `newgrp docker`

# Building

Run `make` to obtain `addrconv` and `svc` binaries.  Run `make image` to
obtain a docker image of the service which listens on port 3000.  Run `make run`
to run the service.

# Packages

### `cmd/addrconv`

`addrconv` is a basic command line tool for converting between address formats.

### `cmd/svc`

`svc` is a small microservice which will convert a provided address into all
three currently used Bitcoin Cash address formats.

### `cashaddress`

Allows for encoding and decoding cashaddresses

### `legacy`

Allows for encoding and decoding legacy addresses (including copay).

### `address`

Provides a generic struct which allows decoding and encoding into cashaddress,
legacy, and copay formats.

### `static`

Static HTML assets required by the cmd/svc's optional main page.  The
javascript interacts with the provided api.

# References

* [Specification](https://github.com/Bitcoin-UAHF/spec/blob/master/cashaddr.md)
* [Chris Pacia's Go Implementation](https://github.com/cpacia/bchutil/)
* [GopherJS Converter](https://github.com/cashaddress/cashaddress.github.io)
* [Javascript](https://github.com/bitcoincashjs/cashaddrjs)
* [Bitpay's Converter](https://github.com/bitpay/address-translator)
* [Python](https://github.com/fyookball/electrum/blob/master/lib/cashaddr.py)
* [C++ Part 1](https://github.com/Bitcoin-ABC/bitcoin-abc/blob/master/src/cashaddrenc.cpp), [C++ Part 2](https://github.com/Bitcoin-ABC/bitcoin-abc/blob/master/src/cashaddr.cpp), [Base32 Functions](https://github.com/Bitcoin-ABC/bitcoin-abc/blob/master/src/utilstrencodings.cpp), [ConvertBits Function](https://github.com/Bitcoin-ABC/bitcoin-abc/blob/26bef497950b4d499f8a9e32f42d9cf6439f089f/src/utilstrencodings.h#L152)
