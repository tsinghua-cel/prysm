#!/usr/bin/env sh
docker build --no-cache -t tscel/bf.beacon:v7.1.0 -f Dockerfile.beacon .
docker build --no-cache -t tscel/bf.validator:v7.1.0 -f Dockerfile.validator .
docker push tscel/bf.beacon:v7.1.0
docker push tscel/bf.validator:v7.1.0



docker build --no-cache -t tscel/bf.prysm:v7.1.0 -f Dockerfile.prysm .
docker build --no-cache -t tscel/prysmctl:v7.1.0 -f Dockerfile.prysmctl .
docker push tscel/bf.prysm:v7.1.0
docker push tscel/prysmctl:v7.1.0