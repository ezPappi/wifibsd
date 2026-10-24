VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath
DIST := dist

.PHONY: build test netbsd dist clean

build:
	go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o netcli ./cmd/netcli

test:
	go vet ./...
	GOOS=netbsd go vet ./...
	go test ./...

# Static binaries for NetBSD. 386 uses software floating point so it runs
# on CPUs without SSE2 (e.g. AMD Geode on Soekris net4801/net5501).
netbsd:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=netbsd GOARCH=amd64 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(DIST)/netcli-netbsd-amd64 ./cmd/netcli
	CGO_ENABLED=0 GOOS=netbsd GOARCH=386 GO386=softfloat go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(DIST)/netcli-netbsd-386 ./cmd/netcli
	CGO_ENABLED=0 GOOS=netbsd GOARCH=arm64 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(DIST)/netcli-netbsd-arm64 ./cmd/netcli
	CGO_ENABLED=0 GOOS=netbsd GOARCH=arm GOARM=6 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(DIST)/netcli-netbsd-armv6 ./cmd/netcli

dist: netbsd
	tar -czf $(DIST)/netcli-$(VERSION)-netbsd.tar.gz -C $(DIST) $$(cd $(DIST) && ls netcli-netbsd-*) \
		-C $(CURDIR) contrib examples README.md

clean:
	rm -rf netcli $(DIST)
