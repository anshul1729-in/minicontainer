ALPINE_URL=https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.3-x86_64.tar.gz

# 1. Downloads and extracts Alpine Linux into ./alpine-rootfs
rootfs:
	mkdir -p alpine-rootfs
	curl -Lo alpine.tar.gz $(ALPINE_URL)
	tar -xzf alpine.tar.gz -C alpine-rootfs
	rm -f alpine.tar.gz
	@echo "Alpine rootfs ready!"

# 2. Compiles main.go into the minicontainer binary
build:
	go build -o minicontainer main.go

# 3. Builds and starts an isolated Alpine shell
run: build
	sudo ./minicontainer run /bin/sh