# SPTS

![Go](https://github.com/z0rr0/spts/workflows/Go/badge.svg)
![Version](https://img.shields.io/github/tag/z0rr0/spts.svg)
![License](https://img.shields.io/github/license/z0rr0/spts.svg)

Speed Test Service.

It's a simple client-server application for measuring network speed.

It uses only TCP protocol for communication between client and server.
It works like `curl` commands (but without HTTP protocol and `data.log` file saving, random bytes are used instead):

```sh
curl -o data.log -w "speed download: %{speed_download}\n" \
  http://localhost:28082/download

curl -X POST --data-binary @data.log -w "speed upload: %{speed_upload}\n" \
  http://localhost:28082/upload
```

**IMPORTANT NOTE**: It doesn't work correctly with reverse proxies (like nginx) due to payload buffering.

## Build and test

```sh
make all
```

## Usage

```
Usage of spts:
  -clients int
        max clients (for server mode) (default 1)
  -debug
        enable debug mode
  -dot
        show dot progress output (for client mode)
  -host string
        host to listen on for server mode or connect to for client mode (default "localhost")
  -port value
        port to listen on (integer in range 1..65535)
  -server
        run in server mode
  -timeout duration
        timeout for requests (half for client mode) (default 3s)
  -version
        print version and exit
```

Client run example:

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
export SPTS_TOKENS="1:token1,2:token2"

# client
export SPTS_KEY="1:token1"

# where 
#   - token1 and token2 are some random hex strings
#   - 1 and 2 are client ID examples, any uint16 values in range 1..65535
```

Example how to generate random HEX token for authorization:

```sh
# read random 32 bytes
head -c 32 /dev/urandom| xxd -p -c 64
```

The application doesn't encrypt data, because there is no sensitive information (only random bytes).
But authorization token is not sent in plain text.
Common secrets are used to generate sha512 hash signature
which will be encoded in base64 format and added to HTTP header.

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

## License

This source code is governed by a [MIT](https://opensource.org/license/mit/)
license that can be found in the [LICENSE](https://github.com/z0rr0/spts/blob/master/LICENSE) file.
