# SPTS

![Go](https://github.com/z0rr0/spts/workflows/Go/badge.svg)
![Version](https://img.shields.io/github/tag/z0rr0/spts.svg)
![License](https://img.shields.io/github/license/z0rr0/spts.svg)

Speed Test Service.

It's a simple client-server application for measuring network speed.

It uses only HTTP protocol for communication between client and server.
It works like `curl` commands (but without `data.log` file saving, random bytes are used instead):

```sh
curl -o data.log -w "speed download: %{speed_download}\n" \
  http://localhost:28082/download

curl -X POST --data-binary @data.log -w "speed upload: %{speed_upload}\n" \
  http://localhost:28082/upload
```

## Build and test

```sh
make all
```

## Usage

```
Usage of ./spts:
  -debug
        enable debug mode
  -host string
        host (listen on for server, connect to for client) (default "localhost")
  -nodot
        disable dot output (for client mode)
  -port uint
        port to listen on (default 28082)
  -server
        run in server mode
  -timeout duration
        timeout for requests (double value for client) (default 3s)
  -version
        print version and exit
```

Run client:

```sh
./spts -host 192.168.1.76

IP address:     192.168.1.88
Download speed: 48.51 MBits/s
Upload speed:   78.13 MBits/s
```

### Authorization

It's supported Bearer token authorization for server and client using environment variables:

```sh
# server with multiple tokens
export SPTS_TOKENS="token1,token2"

# client
export SPTS_KEY="token1"
```

Example to generate random token:

```sh
head -c 32 /dev/urandom| base64
```

### Docker

Build image:

```sh
make docker
```

Use prepared:

```sh
# server
docker run --rm --name spts -p 28082:28082 z0rr0/spts:latest -debug -host 0.0.0.0 -server
# or as a daemon with authorization (file $PWD/env), memory and logs limitations
docker run -d --name spts -m 32m -p 28082:28082 --env-file=$PWD/env \
  --log-opt max-size=10m --restart unless-stopped \
  z0rr0/spts:latest -debug -host 0.0.0.0 -server

# client
docker run --rm --name spts_client z0rr0/spts:latest -host $SERVER
```