# SPTS

![Go](https://github.com/z0rr0/spts/workflows/Go/badge.svg)
![Version](https://img.shields.io/github/tag/z0rr0/spts.svg)
![License](https://img.shields.io/github/license/z0rr0/spts.svg)

Speed Test Service.

It's a simple client-server application for measuring network speed.

It uses only HTTP protocol for communication between client and server.
It works like `curl` commands (but without `data.log` file saving, random bytes are used instead):

```shell
curl -o data.log -w "speed download: %{speed_download}\n" http://localhost:18081/download
curl -X POST --data-binary @data.log -w "speed upload: %{speed_upload}\n"  http://localhost:18081/upload
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
        port to listen on (default 18081)
  -server
        run in server mode
  -timeout duration
        timeout for requests (double value for client) (default 3s)
  -version
        print version and exit
```
