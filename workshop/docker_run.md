```sh
# Build the building image
make .bin-image

# Run the image with mounted code volume and network connections
docker run -v ".:/src" -v "./build:/out"  --network=host  blockbook-build

# Look at running containers
docker ps

# Get the container ID for the blockbook-build container
CONTAINER_ID=$(docker ps -q --filter ancestor=blockbook-build)

# Get a shell in the container
docker exec -it $CONTAINER_ID /bin/bash

# Full copyable command
docker exec -it $(docker ps -q --filter ancestor=blockbook-build) /bin/bash

---

# INSIDE THE CONTAINER

# Go to the source code directory
cd /src
# Build the main binary
go build
# Regenerate config
./contrib/scripts/build-blockchaincfg.sh bitcoin_regtest
# Run the app ... logs should be visible in the terminal
./blockbook -sync -blockchaincfg=build/blockchaincfg.json -internal=:9030 -public=:9130 -logtostderr -enablesubnewtx
# Visit http://localhost:9130/ in the browser

# LOOP: Now you can modify the code locally and always rebuild and run the app in the container

---

# Stop the container
docker stop $CONTAINER_ID
```
