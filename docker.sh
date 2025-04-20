#!/usr/bin/env sh
docker build --no-cache -t tscel/bf.prysm:v5.3.3 -f Dockerfile.prysm .
docker build --no-cache -t tscel/prysmctl:v5.3.3 -f Dockerfile.prysmctl .

#docker push tscel/bf.prysm:v5.3.3
#docker push tscel/prysmctl:v5.3.3
