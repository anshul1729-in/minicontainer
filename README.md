# minicontainer

A lightweight Linux container runtime built from scratch in Go using raw Linux kernel primitives—without Docker, containerd, or LXC.

## Overview

Many developers treat containers like lightweight virtual machines, but under the hood, a container is simply a normal Linux process with blinders and resource handcuffs applied by the kernel. I built `minicontainer` to strip away the abstraction of Docker and interact directly with the Linux kernel using Go's `syscall` package.

---

## Core Linux Kernel Primitives Used

This project isolates a process using three fundamental mechanisms in the Linux kernel:

### 1. Namespaces (Controlling What a Process Can *See*)
When `minicontainer` starts a container, it passes clone flags to the kernel to isolate the process from the host system:
* **PID Namespace (`CLONE_NEWPID`):** Gives the container its own process ID tree starting at `PID 1`, preventing it from seeing or signaling host processes.
* **UTS Namespace (`CLONE_NEWUTS`):** Isolates the hostname so setting the hostname to `alpine-box` inside the container does not rename the host machine.
* **Mount Namespace (`CLONE_NEWNS`):** Gives the container an independent mount table with private propagation (`MS_PRIVATE`) so filesystem mounts inside the container never leak back to the host.
* **Network Namespace (`CLONE_NEWNET`):** Isolates network interfaces, giving the container its own private loopback interface separated from the host network card.

### 2. Filesystem Isolation (`pivot_root` + Alpine Linux Minirootfs)
An isolated process needs basic Linux utilities (`sh`, `ps`, `ls`) to function, so `minicontainer` uses a ~3 MB Alpine Linux root filesystem (`alpine-rootfs`). Instead of using `chroot` (which only changes the root directory pointer and leaves the host filesystem mounted), this runtime uses `pivot_root` to swap the root mount and completely unmount the old host filesystem (`MNT_DETACH`). It then mounts a fresh `/proc` virtual filesystem so tools like `ps aux` only report container processes.

### 3. Resource Limits via Cgroups v2 (Controlling What a Process Can *Use*)
To prevent a noisy or buggy container from starving the host machine, `minicontainer` writes directly to the unified **cgroups v2** hierarchy at `/sys/fs/cgroup/minicontainer`:
* **RAM Limit (`memory.max`):** Capped at **50 MB** (`52428800` bytes) with swap disabled (`memory.swap.max = 0`).
* **CPU Limit (`cpu.max`):** Capped at **50% of one CPU core** (`50000` µs quota per `100000` µs period).
* **Process Attachment (`cgroup.procs`):** Attaches the container's host PID to enforce the limits immediately.

---

## Architecture: How the `/proc/self/exe` Re-Execution Works

Because the Linux kernel only places *newly spawned child processes* into a newly created `CLONE_NEWPID` namespace, `minicontainer` executes in two stages:

```text
Host OS (WSL2 / Linux)
  │
  ├─► 1. Parent Process: sudo ./minicontainer run /bin/sh
  │      ├── Requests CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWNET
  │      ├── Writes 50MB RAM & 50% CPU limits to /sys/fs/cgroup/minicontainer
  │      └── Re-executes its own binary via /proc/self/exe with "child" arg
  │
  └─► 2. Child Process: /proc/self/exe child /bin/sh (Runs as PID 1 in container)
         ├── Sets container hostname to "alpine-box"
         ├── Swaps root filesystem to ./alpine-rootfs using pivot_root()
         ├── Mounts fresh /proc virtual filesystem
         └── Launches /bin/sh inside the isolated Alpine environment
```

---

## Project Structure

```text
minicontainer/
├── .gitignore    # Excludes alpine-rootfs/ and compiled binaries from Git
├── go.mod        # Go module definition
├── main.go       # Container runtime implementation (namespaces, pivot_root, cgroups v2)
├── Makefile      # Automation for downloading Alpine rootfs, building, and running
└── README.md     # Project documentation
```

---

## Getting Started

### Prerequisites
* Linux or Windows **WSL 2** (with cgroups v2 enabled)
* **Go** 1.20+
* `make` and `curl`

### 1. Download the Alpine Linux Root Filesystem
```bash
make rootfs
```

### 2. Build and Launch the Container
```bash
make run
```

### 3. Verify Isolation Inside the Container
```text
$ make run
go build -o minicontainer main.go
sudo ./minicontainer run /bin/sh
[Container] Running [/bin/sh] as PID 1 inside isolated namespace
/ # ps aux
PID   USER     TIME  COMMAND
    1 root      0:00 /proc/self/exe child /bin/sh
    7 root      0:00 /bin/sh
    8 root      0:00 ps aux
/ # hostname
alpine-box
/ # exit
```