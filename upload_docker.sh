#!/usr/bin/env bash

TAG=$(git tag | sort -V | tail -1)
VERSION="${TAG:1}"

echo "version: ${VERSION}"

# tag version
docker tag github.com/z0rr0/spts:latest github.com/z0rr0/spts:${VERSION}

# push
docker push github.com/z0rr0/spts:${VERSION}
docker push github.com/z0rr0/spts:latest
