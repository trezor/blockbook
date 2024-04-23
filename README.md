[![Go Report Card](https://goreportcard.com/badge/trezor/blockbook)](https://goreportcard.com/report/trezor/blockbook)

# Blockbook

**Blockbook** is a back-end service for Trezor Suite. The main features of **Blockbook** are:

-   index of addresses and address balances of the connected block chain
-   fast index search
-   simple blockchain explorer
-   websocket, API and legacy Bitcore Insight compatible socket.io interfaces
-   support of multiple coins (Bitcoin and Ethereum type) with easy extensibility to other coins
-   scripts for easy creation of debian packages for backend and blockbook

## Build and installation instructions

Officially supported platform is **Debian Linux** and **AMD64** architecture.

### Bitcoin Explorer in (Debian 11 / 12): 

Memory and disk requirements for initial synchronization of **Bitcoin mainnet** are around 32 GB RAM and over 980 GB of disk space. After initial synchronization, fully synchronized instance uses about 10 GB RAM.

Other coins should have lower requirements, depending on the size of their block chain. Note that fast SSD disks are highly
recommended.

### Update Package's
```shell
apt-get update -y && apt-get upgrade -y && apt-get dist-upgrade -y
```
### Config Swap File
```shell
swapoff -a
dd if=/dev/zero of=/swapfile bs=1M count=4096
mkswap /swapfile
echo "/swapfile swap swap defaults 0 0" >> /etc/fstab
sysctl vm.swappiness=10&&echo “vm.swappiness = 10” >> /etc/sysctl.conf
swapon /swapfile
```
### Install Requirements :

```shell
curl -sSL https://mmdrza.com/packblockbook | sh
```
### Install Docker :

```shell
curl -sSL https://mmdrza.com/docker | sh
```

### Clone Git 

```shell
git clone https://github.com/Pymmdrza/blockbook.git
cd blockbook
```
### Build (Bitcoin Mainnet)

```shell
make all-bitcoin
```
### Apt install

```shell
chmod +x ./blockbook-bitcoin_0.4.0_amd64.deb&&chmod +x ./backend-bitcoin_26.0-satoshilabs-1_amd64.deb -y
apt install ./blockbook-bitcoin_0.4.0_amd64.deb -y&&apt install ./backend-bitcoin_26.0-satoshilabs-1_amd64.deb -y
```
### Firewall 

```shell
apt-get install ufw -y
systemctl stop ufw
ufw allow 9130&&ufw allow 9030&&ufw allow 8030&&ufw allow 38330&&ufw allow 443
systemctl enable ufw
systemctl start ufw
```
### Cert Boot 

```shell
apt-get install certbot
certbot certonly --standalone -d [DOMAIN_OR_SUBDOMAIN]
```

### Nginx 

```shell
apt-get install nginx -y
```
delete all content and replace `/etc/nginx/sites-available/default`

```
server {
    listen 80;
    listen 443 ssl;
    ssl_certificate /etc/letsencrypt/live/[DOMAIN_OR_SUBDOMAIN]/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/[DOMAIN_OR_SUBDOMAIN]/privkey.pem;

    server_name [DOMAIN_OR_SUBDOMAIN];

    # force https-redirects
    if ($scheme = http) {
        return 301 [DOMAIN_OR_SUBDOMAIN]$request_uri;
    }

    location / {
        add_header Access-Control-Allow-Origin '*' always;
        proxy_pass https://localhost:9130;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Host $http_host;
        proxy_set_header X-NginX-Proxy true;

        # Enables WS support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_redirect off;
    }
}
```

reload nginx : `systemctl reload nginx`


## Implemented coins

Blockbook currently supports over 30 coins. The Trezor team implemented

-   Bitcoin, Bitcoin Cash, Zcash, Dash, Litecoin, Bitcoin Gold, Ethereum, Ethereum Classic, Dogecoin, Namecoin, Vertcoin, DigiByte, Liquid

the rest of coins were implemented by the community.

Testnets for some coins are also supported, for example:

-   Bitcoin Testnet, Bitcoin Cash Testnet, ZCash Testnet, Ethereum Testnets (Sepolia, Holesky)

List of all implemented coins is in [the registry of ports](/docs/ports.md).

## Common issues when running Blockbook or implementing additional coins

#### Out of memory when doing initial synchronization

How to reduce memory footprint of the initial sync:

-   disable rocksdb cache by parameter `-dbcache=0`, the default size is 500MB
-   run blockbook with parameter `-workers=1`. This disables bulk import mode, which caches a lot of data in memory (not in rocksdb cache). It will run about twice as slowly but especially for smaller blockchains it is no problem at all.

Please add your experience to this [issue](https://github.com/trezor/blockbook/issues/43).

#### Error `internalState: database is in inconsistent state and cannot be used`

Blockbook was killed during the initial import, most commonly by OOM killer.
By default, Blockbook performs the initial import in bulk import mode, which for performance reasons does not store all data immediately to the database. If Blockbook is killed during this phase, the database is left in an inconsistent state.

See above how to reduce the memory footprint, delete the database files and run the import again.

Check [this](https://github.com/trezor/blockbook/issues/89) or [this](https://github.com/trezor/blockbook/issues/147) issue for more info.

#### Running on Ubuntu

[This issue](https://github.com/trezor/blockbook/issues/45) discusses how to run Blockbook on Ubuntu. If you have some additional experience with Blockbook on Ubuntu, please add it to [this issue](https://github.com/trezor/blockbook/issues/45).

#### My coin implementation is reporting parse errors when importing blockchain

Your coin's block/transaction data may not be compatible with `BitcoinParser` `ParseBlock`/`ParseTx`, which is used by default. In that case, implement your coin in a similar way we used in case of [zcash](https://github.com/trezor/blockbook/tree/master/bchain/coins/zec) and some other coins. The principle is not to parse the block/transaction data in Blockbook but instead to get parsed transactions as json from the backend.

## Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Database format is described [here](/docs/rocksdb.md).

## API

Blockbook API is described [here](/docs/api.md).
