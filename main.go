//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	rootfsPath = "./alpine-rootfs"
	cgroupPath = "/sys/fs/cgroup/minicontainer"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: sudo ./minicontainer run <command> [args...]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		parent()
	case "child":
		child()
	default:
		panic("Unknown command. Use 'run'")
	}
}

// 1. PARENT: Runs on the host, sets up namespaces & cgroups v2, and spawns "child"
func parent() {
	args := append([]string{"child"}, os.Args[2:]...)
	cmd := exec.Command("/proc/self/exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Create isolated UTS (hostname), PID, Mount (NS), and Network namespaces
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	if err := cmd.Start(); err != nil {
		panic(fmt.Sprintf("Failed to start container process: %v", err))
	}

	// Configure cgroups v2 limits using the child's host PID
	setupCgroupsV2(cmd.Process.Pid)
	defer cleanupCgroup()

	_ = cmd.Wait()
}

// 2. CHILD: Wakes up as PID 1 inside the isolated namespaces
func child() {
	fmt.Printf("[Container] Running %v as PID %d inside isolated namespace\n", os.Args[2:], os.Getpid())

	// Set isolated container hostname (UTS namespace)
	must(syscall.Sethostname([]byte("alpine-box")))

	// Ensure mount propagation is private so host mounts stay untouched
	must(syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""))

	// Swap root filesystem using pivot_root
	must(pivotRoot(rootfsPath))

	// Mount isolated /proc filesystem so 'ps' and 'top' only see container PIDs
	must(syscall.Mount("proc", "/proc", "proc", 0, ""))
	defer syscall.Unmount("/proc", 0)

	// Execute the user-requested binary inside the Alpine filesystem
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Command exited with error: %v\n", err)
	}
}

// Swaps the root mount to newRoot and unmounts the host filesystem completely
func pivotRoot(newRoot string) error {
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount failed (ensure ./alpine-rootfs exists): %w", err)
	}

	putOld := filepath.Join(newRoot, ".pivot_root")
	if err := os.MkdirAll(putOld, 0700); err != nil {
		return err
	}

	if err := syscall.PivotRoot(newRoot, putOld); err != nil {
		return err
	}

	if err := os.Chdir("/"); err != nil {
		return err
	}

	putOldInContainer := "/.pivot_root"
	if err := syscall.Unmount(putOldInContainer, syscall.MNT_DETACH); err != nil {
		return err
	}
	return os.RemoveAll(putOldInContainer)
}

// Writes directly to /sys/fs/cgroup (cgroups v2 unified hierarchy)
func setupCgroupsV2(pid int) {
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		fmt.Printf("[Warning] Could not create cgroup dir: %v\n", err)
		return
	}

	// Cap RAM at 50 MB (52,428,800 bytes)
	_ = os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte("52428800"), 0644)
	_ = os.WriteFile(filepath.Join(cgroupPath, "memory.swap.max"), []byte("0"), 0644)

	// Cap CPU at 50% of 1 core (50,000us quota per 100,000us period)
	_ = os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte("50000 100000"), 0644)

	// Attach container host PID to this cgroup
	_ = os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644)
}

func cleanupCgroup() {
	_ = os.Remove(cgroupPath)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}