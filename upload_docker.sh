#!/usr/bin/env bash

TAG=$(git tag | sort -V | tail -1)
VERSION="${TAG:1}"

echo "version: ${VERSION}"

# tag version
docker tag z0rr0/spts:latest z0rr0/spts:${VERSION}

# push
docker push z0rr0/spts:${VERSION}
docker push z0rr0/spts:latest
