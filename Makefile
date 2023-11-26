TARGET=spts
TS=$(shell date -u +"%FT%T")
TAG=$(shell git tag | sort -V | tail -1)
COMMIT=$(shell git log --oneline | head -1)
VERSION=$(firstword $(COMMIT))
LDFLAGS=-X main.Version=$(TAG) -X main.Revision=git:$(VERSION) -X main.BuildDate=$(TS)
DOCKER_TAG=z0rr0/spts

# coverage check
# go tool cover -html=coverage.out

all: test

build:
	go build -o $(PWD)/$(TARGET) -ldflags "$(LDFLAGS)"

fmt:
	gofmt -d .

check_fmt:
	@test -z "`gofmt -l .`" || { echo "ERROR: failed gofmt, for more details run - make fmt"; false; }
	@-echo "gofmt successful"

lint: check_fmt
	go vet $(PWD)/...
	-golangci-lint run $(PWD)/...

test: build lint
	go test -race -cover $(PWD)/...

docker: lint clean
	docker build --build-arg LDFLAGS="$(LDFLAGS)" -t $(DOCKER_TAG) .

docker_both: lint clean
	docker buildx build --platform linux/amd64,linux/arm64 --build-arg LDFLAGS="$(LDFLAGS)" -t $(DOCKER_TAG) .

docker_linux_amd64: lint clean
	docker buildx build --platform linux/amd64 --build-arg LDFLAGS="$(LDFLAGS)" -t $(DOCKER_TAG) .

clean:
	rm -f $(PWD)/$(TARGET)
	find ./ -type f -name "*.out" -delete
