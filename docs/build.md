# Blockbook Build Guide

## Setting up your development environment

Supported environment to develop Blockbook is Linux. Although it is possible build and run Blockbook on macOS
or Windows our build process is not prepared for it. But you can still build Blockbook [manually](#manual-build).

The only dependency required to build Blockbook is Docker. You can see how to install Docker [here](https://docs.docker.com/install/linux/docker-ce/debian/).
Manual build require additional dependencies that are described in appropriate section.

## Build in Docker environment

All build operations run in Docker container in order to keep build environment isolated. Makefile in root of repository
defines few targets used for building, testing and packaging of Blockbook. With Docker image definitions and Debian
package templates in *build/docker* and *build/templates* respectively, they are only inputs that make build process.

Docker build images are created at first execution of Makefile and that information is persisted. (Actually there are
created two files in repository – .bin-image and .deb-image – that are used as tags.) Sometimes it is necessary to
rebuild Docker images, it is possible by executing `make build-images`.

### Building binary

Just run `make` and that is it. Output binary is stored in *build* directory. Note that although Blockbook is Go application
it is dynamically linked with RocksDB dependencies and ZeroMQ. Therefore operating system where Blockbook will be
executed still need that dependencies installed. See [Manual build](#manual-build) instructions below or install
Blockbook via Debian packages.

### Building debug binary

Standard binary contains no debug symbols. Execute `make build-debug` to get binary for debugging.

### Testing

How to execute tests is described in separate document [here](/docs/testing.md).

### Building Debian packages

Blockbook and particular coin back-end are usually deployed together. They are defined in same place as well.
So typical way to build Debian packages is build Blockbook and back-end deb packages by single command. But it is not
mandatory, of course.

> Early releases of Blockbook weren't so friendly for extending. One had to define back-end package, Blockbook package,
> back-end configuration and Blockbook configuration as well. There were many options that were duplicated across
> configuration files and therefore error prone.
>
> Now, all configuration options and also build options for both Blockbook and back-end are defined in single JSON
> file and all stuff required during build is generated dynamically.

Makefile targets follow simple pattern, there are a few prefixes that define what to build.

* *deb-blockbook-&lt;coin&gt;* – Build Blockbook package for given coin.

* *deb-backend-&lt;coin&gt;* – Build back-end package for given coin.

* *deb-&lt;coin&gt;* – Build both Blockbook and back-end packages for given coin.

* *all-&lt;coin&gt;* – Similar to deb-&lt;coin&gt; but clean repository and rebuild Docker image before package build. It is useful
  for production deployment.

* *all* – Build both Blockbook and back-end packages for all coins.

Which coins are possible to build is defined in *configs/coins*. Each coin has to have JSON config file there.

For example we want to build some packages for Bitcoin and Bitcoin Testnet.

```bash
# make all-bitcoin deb-blockbook-bitcoin_testnet
...
# ls build/*.deb
build/backend-bitcoin_0.21.0-satoshilabs-1_amd64.deb  build/blockbook-bitcoin_0.3.5_amd64.deb  build/blockbook-bitcoin-testnet_0.3.5_amd64.deb
```

We have built one back-end package, for Bitcoin, and two Blockbook packages, for Bitcoin and Bitcoin Testnet. The `all-bitcoin` initially cleaned the build directory and rebuilt the Docker build image.

### Extra variables

There are few variables that can be passed to `make` in order to modify build process:

`BASE_IMAGE`: Specifies the base image of the Docker build image. By default, it chooses the same Linux distro as the host machine but you can override it this way `make BASE_IMAGE=debian:10 all-bitcoin` to make a build for Debian 10.

*Please be aware that we are currently running our Blockbooks on Debian 11 and do not offer support with running it on other distros.*

`NO_CACHE`: Common behaviour of Docker image build is that build steps are cached and next time they are executed much faster.
Although this is a good idea, when something went wrong you will need to override this behaviour somehow. Execute this
command: `make NO_CACHE=true all-bitcoin`.

`TCMALLOC`: RocksDB, the storage engine used by Blockbook, allows to use alternative memory allocators. Use the `TCMALLOC` variable to specify Google's TCMalloc allocator `make TCMALLOC=true all-bitcoin`. To run Blockbook built with TCMalloc, the library must be installed on the target server, for example by `sudo apt-get install google-perftools`.

`PORTABLE`: By default, the RocksDB binaries shipped with Blockbook are optimized for the platform you're compiling on (-march=native or the equivalent). If you want to build a portable binary, use `make PORTABLE=1 all-bitcoin`.

### Naming conventions and versioning

All configuration keys described below are in coin definition file in *configs/coins*.

**install and data directories**

Both Blockbook and back-end have separated install and data directories. They use common preffix and are defined in
*configs/environ.json* and all templates use them.

* back-end install directory is */opt/coins/nodes/&lt;coin&gt;*.
* back-end data directory is */opt/coins/data/&lt;coin&gt;/backend*.
* Blockbook install directory is */opt/coins/blockbook/&lt;coin&gt;*.
* Blockbook data directory is */opt/coins/data/&lt;coin&gt;/blockbook*.

*coin* used above is defined in *coin.alias* in coin definition file.

**package names**

Package names are defined in *backend.package_name* and *blockbook.package_name* in coin definition file. We use
simple pattern *&lt;prefix&gt;-&lt;coin&gt;* to name packages where *prefix* is either *blockbook* or *backend* and
*coin* is made similarly to *coin.alias*. We use convention that coin name uses lowercase characters and dash '-' as
a word delimiter. Testnet versions of coins must have *-testnet* suffix. That differs from *coin.alias* because
underscore has a special meaning in Debian packaging. For example there are packages *backend-bitcoin* and
*blockbook-bitcoin-testnet*.

**user names**

User names are defined in *backend.system_user* and *blockbook.system_user* in coin definition file. We follow common
Linux conventions, user names use lowercase characters and dash '-' as a word delimiter.

Back-end user name use coin name only, including testnet services. For example there is *bitcoin* user for both
*backend-bitcoin* and *backend-bitcoin-testnet* packages.

Blockbook user name has *blockbook-* prefix and coin name (made same as back-end version). For example there is
*blockbook-bitcoin*  user for both *blockbook-bitcoin* and *blockbook-bitcoin-testnet* packages.

**back-end versioning**

Since we have to distinguish version of coin distribution and version of our configuration we follow standard Debian
package versioning rules (for details see
[Debian policy](https://www.debian.org/doc/debian-policy/ch-controlfields.html#version)). There is upstream version
and revision both defined in coin definition file in *backend.version* and *backend.package_revision*, respectively.

**blockbook versioning**

Blockbook versioning is much simpler. There is only one version defined in *configs/environ.json*.

### Back-end building

Because we don't keep back-end archives inside our repository we download them during build process. Build steps
are these: download, verify and extract archive, prepare distribution and make package.

All configuration keys described below are in coin definition file in *configs/coins*.

**download archive**

URL from where is archive downloaded is defined in *backend.binary_url*.

**verify archive**

There are three different approaches how is archive verification done. Some projects use PGP sign of archive, some
have signed sha256 sums and some don't care about verification at all. So there is option *backend.verification_type* that
could be *gpg*, *gpg-sha256* or *sha256* and chooses particular method.

*gpg* type require file with digital sign and maintainer's public key imported in Docker build image (see below). Sign
file is downloaded from URL defined in *backend.verification_source*. Than is passed to gpg in order to verify archive.

*gpg-sha256* type require signed checksum file and maintainer's public key imported in Docker build image (see below).
Checksum file is downloaded from URL defined in *backend.verification_source*. Then is verified by gpg and passed to
sha256sum in order to verify archive.

*sha256* type is used for coins that don't support verification at all. In *backend.verification_source* is defined
hexadecimal string that is compared with output of sha256sum. Although this solution is not secure, it avoid download
errors and other surprises at least.

*gpg* and *gpg-sha256* types require maintainer's public key imported in Docker build image. It is not expected that
maintainer's key will change requently while sing or checksum files are changed every release, so it is ideal to
store maintainer's key within image definition. Public keys are stored in *build/docker/deb/gpg-keys* directory. Docker
image must be rebuilt by calling `make build-images`.

**extract archive**

Extraction command is defined in *backend.extract_command*. Content of archive must be extracted to `./backend` directory.
See bitcoin.json and vertcoin.json for different approaches.

**prepare distribution**

There are two steps in this stage – exclude unnecessary files and generate configuration.

Some files are not required for server deployment, some binaries have unnecessary dependencies, so it is good idea to
extract these files from output package. Files to extract are listed in *backend.exclude_files*. Note that paths are
relative to *backend* directory where archive is extracted.

Configuration is described in [config.md](/docs/config.md).

## Manual build

Instructions below are focused on Debian 11 on amd64. If you want to use another Linux distribution or operating system
like macOS or Windows, please adapt the instructions to your target system.

Setup go environment (use newer version of go as available)

```
wget https://golang.org/dl/go1.19.linux-amd64.tar.gz && tar xf go1.19.linux-amd64.tar.gz
sudo mv go /opt/go
sudo ln -s /opt/go/bin/go /usr/bin/go
# see `go help gopath` for details
mkdir $HOME/go
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

Install RocksDB: https://github.com/facebook/rocksdb/blob/master/INSTALL.md
and compile the static_lib and tools. Optionally, consider adding `PORTABLE=1` before the
make command to create a portable binary.

```
sudo apt-get update && sudo apt-get install -y \
    build-essential git wget pkg-config libzmq3-dev libgflags-dev libsnappy-dev zlib1g-dev libzstd-dev  libbz2-dev liblz4-dev
git clone https://github.com/facebook/rocksdb.git
cd rocksdb
git checkout v7.5.3
CFLAGS=-fPIC CXXFLAGS=-fPIC make release
```

Setup variables for grocksdb

```
export CGO_CFLAGS="-I/path/to/rocksdb/include"
export CGO_LDFLAGS="-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4 -lzstd"
```

Install ZeroMQ: https://github.com/zeromq/libzmq

```
git clone https://github.com/zeromq/libzmq
cd libzmq
./autogen.sh
./configure
make
sudo make install
```

Get blockbook sources, install dependencies, build:

```
cd $GOPATH/src
git clone https://github.com/trezor/blockbook.git
cd blockbook
go build
```

### Example command

Blockbook require full node daemon as its back-end. You are responsible for proper installation. Port numbers and
daemon configuration are defined in *configs/coins* and *build/templates/backend/config* directories. You should use
specific installation process for particular coin you want run (e.g. https://bitcoin.org/en/full-node#other-linux-distributions for Bitcoin).

When you have running back-end daemon you can start Blockbook. It is highly recommended use ports described in [ports.md](/docs/ports.md)
for both Blockbook and back-end daemon. You can use *contrib/scripts/build-blockchaincfg.sh* that will generate
Blockbook's blockchain configuration from our coin definition files.

Also, check that your operating system open files limit is set to high enough value - recommended is at least 20000.

Example for Bitcoin:
```
./contrib/scripts/build-blockchaincfg.sh <coin>
./blockbook -sync -blockchaincfg=build/blockchaincfg.json -internal=:9030 -public=:9130 -certfile=server/testcert -logtostderr
```

This command starts Blockbook with parallel synchronization and providing HTTP and Socket.IO interface, with database
in local directory *data* and established ZeroMQ and RPC connections to back-end daemon specified in configuration
file passed to *-blockchaincfg* option.

Blockbook logs to stderr (option *-logtostderr*) or to directory specified by parameter *-log_dir* . Verbosity of logs can be tuned
by command line parameters *-v* and *-vmodule*, for details see https://godoc.org/github.com/golang/glog.

You can check that Blockbook is running by simple HTTP request: `curl https://localhost:9130`. Returned data is JSON with some
run-time information. If the port is closed, Blockbook is syncing data.
