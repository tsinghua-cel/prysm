#!/usr/bin/env sh
docker build --no-cache -t tscel/bf.beacon:v5.1.2 -f Dockerfile.beacon .
docker build --no-cache -t tscel/bf.validator:v5.1.2 -f Dockerfile.validator .
docker build --no-cache -t tscel/prysmctl:v5.1.2 -f Dockerfile.prysmctl .

docker push tscel/bf.beacon:v5.1.2
docker push tscel/bf.validator:v5.1.2
docker push tscel/prysmctl:v5.1.2
