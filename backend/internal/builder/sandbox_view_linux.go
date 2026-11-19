// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package builder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Every child of the LinuxSandbox starts as the sandbox helper: the builder's
// own executable run with SandboxHelperArg. Inside the child's new user,
// mount, network and PID namespaces the helper builds a private view of the
// filesystem — the system directories, the toolchain and the tools read-only,
// the build's own scratch directory writable, nothing else — then restricts
// itself with Landlock to the same, sets the limits and replaces itself with
// the tool. A build therefore cannot read or change another build's files,
// the builder's own files or anything else on the machine (build log #164).

// helperUnavailableExit is the exit status of a helper that cannot isolate
// the child, such as on a kernel without Landlock; with the marker on
// standard error, Run reports ErrSandboxUnavailable.
const helperUnavailableExit = 125

// helperUnavailableMarker starts the helper's message when it cannot isolate.
const helperUnavailableMarker = "plux-sandbox: unavailable: "

// viewSpec is what the helper is told to do.
type viewSpec struct {
	Writable []string `json:"writable"`
	Readable []string `json:"readable"`
	Dir      string   `json:"dir"`
	Path     string   `json:"path"`
	Args     []string `json:"args"`
	CPUTime  uint64   `json:"cpuSeconds,omitempty"`
	Memory   uint64   `json:"memoryBytes,omitempty"`
}

// systemPaths are read-only in every view: the programs and libraries the
// tools need, and the few files of /etc the dynamic linker and the C library
// read. Paths a machine does not have are left out; a symbolic link (such as
// /bin to usr/bin) is kept as a link.
var systemPaths = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32",
	"/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d", "/etc/passwd", "/etc/group", "/etc/nsswitch.conf",
	// The system linker that build scripts are linked with (cc) is chosen
	// through the alternatives links.
	"/etc/alternatives",
}

// devices are the device files every view has, read and written.
var devices = []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"}

// RunSandboxHelper is the helper's entry point; args are the arguments after
// SandboxHelperArg. It returns only on failure, with the exit status.
func RunSandboxHelper(args []string) int {
	// Landlock and no_new_privs bind the calling thread, and the tool
	// replaces this very thread.
	runtime.LockOSThread()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "plux-sandbox: want one argument")
		return 2
	}
	var spec viewSpec
	if err := json.Unmarshal([]byte(args[0]), &spec); err != nil || len(spec.Writable) == 0 || spec.Path == "" {
		fmt.Fprintf(os.Stderr, "plux-sandbox: bad specification: %v\n", err)
		return 2
	}
	if err := enterView(spec); err != nil {
		// The machine refuses what the view needs (a mount, for example under
		// a container's security profile): the build cannot be isolated.
		fmt.Fprintf(os.Stderr, "%s%v\n", helperUnavailableMarker, err)
		return helperUnavailableExit
	}
	if err := setOwnLimits(spec); err != nil {
		fmt.Fprintf(os.Stderr, "plux-sandbox: limits: %v\n", err)
		return 2
	}
	if err := dropCapabilities(); err != nil {
		fmt.Fprintf(os.Stderr, "plux-sandbox: %v\n", err)
		return 2
	}
	if err := restrictWithLandlock(spec); err != nil {
		fmt.Fprintf(os.Stderr, "%s%v\n", helperUnavailableMarker, err)
		return helperUnavailableExit
	}
	env := append(os.Environ(), "TMPDIR="+filepath.Join(spec.Writable[0], "tmp"))
	err := syscall.Exec(spec.Path, append([]string{spec.Path}, spec.Args...), env) //nolint:gosec // G204: the builder's own command, passed by the sandbox.
	fmt.Fprintf(os.Stderr, "plux-sandbox: start %s: %v\n", spec.Path, err)
	return 127
}

// enterView makes the child's root a new, read-only directory that holds
// only the system paths, the readable and writable directories (each at its
// own path) and a few devices, and changes into spec.Dir.
func enterView(spec viewSpec) error {
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make the mounts private: %w", err)
	}
	for _, d := range []string{filepath.Join(spec.Writable[0], "tmp"), filepath.Join(spec.Writable[0], ".sandbox-root")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	root := filepath.Join(spec.Writable[0], ".sandbox-root")
	if err := syscall.Mount("tmpfs", root, "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, "mode=0750,size=1m"); err != nil {
		return fmt.Errorf("mount the root: %w", err)
	}
	if err := populateView(root, spec); err != nil {
		return err
	}
	if err := pivotInto(root); err != nil {
		return err
	}
	if err := syscall.Chdir(spec.Dir); err != nil {
		return fmt.Errorf("change into %s: %w", spec.Dir, err)
	}
	return nil
}

// populateView binds into root what the child may see, and mounts its own
// /proc.
func populateView(root string, spec viewSpec) error {
	for _, p := range append(slices.Clone(systemPaths), spec.Readable...) {
		if err := bindInto(root, p, true); err != nil {
			return err
		}
	}
	for _, p := range append(slices.Clone(spec.Writable), devices...) {
		if err := bindInto(root, p, false); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o750); err != nil {
		return fmt.Errorf("create /proc: %w", err)
	}
	// The child is the first process of its own PID namespace, so this
	// /proc shows only the child and what it starts.
	if err := syscall.Mount("proc", filepath.Join(root, "proc"), "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}
	return maskProc(filepath.Join(root, "proc"))
}

// maskedProc are the files and directories of /proc that describe the whole
// machine rather than the child's processes; the view hides them as Docker
// hides them from a container. The rest of /proc is read-only to the child
// (Landlock grants it nothing else there), which covers the paths Docker
// makes read-only (/proc/sys, /proc/sysrq-trigger, /proc/bus, /proc/fs and
// /proc/irq).
var maskedProc = []string{
	"acpi", "asound", "interrupts", "kcore", "keys", "latency_stats",
	"sched_debug", "scsi", "timer_list", "timer_stats",
}

// maskProc hides maskedProc in the /proc mounted at dir: an empty read-only
// directory over a directory, /dev/null, read-only, over a file. The tool
// cannot remove them, since Landlock refuses it every mount change.
func maskProc(dir string) error {
	for _, name := range maskedProc {
		p := filepath.Join(dir, name)
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("look at /proc/%s: %w", name, err)
		}
		if info.IsDir() {
			if err := syscall.Mount("tmpfs", p, "tmpfs", syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, "mode=0555"); err != nil {
				return fmt.Errorf("hide /proc/%s: %w", name, err)
			}
			continue
		}
		if err := syscall.Mount("/dev/null", p, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("hide /proc/%s: %w", name, err)
		}
		if err := syscall.Mount("", p, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
			return fmt.Errorf("make /proc/%s read-only: %w", name, err)
		}
	}
	return nil
}

// pivotInto makes root the child's root, drops the machine's root from its
// view and makes the new root read-only.
func pivotInto(root string) error {
	old := filepath.Join(root, ".old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", old, err)
	}
	if err := syscall.PivotRoot(root, old); err != nil {
		return fmt.Errorf("pivot to the view: %w", err)
	}
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("enter the view: %w", err)
	}
	if err := syscall.Unmount("/.old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("leave the old root: %w", err)
	}
	if err := os.Remove("/.old"); err != nil {
		return fmt.Errorf("remove the old root's mount point: %w", err)
	}
	if err := syscall.Mount("", "/", "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
		return fmt.Errorf("make the root read-only: %w", err)
	}
	return nil
}

// bindInto makes path, from the machine, appear at the same path below root:
// a directory or file is bound, read-only when readOnly, a symbolic link is
// copied, and a path the machine does not have is left out.
func bindInto(root, path string, readOnly bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look at %s: %w", path, err)
	}
	dst := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("read the link %s: %w", path, err)
		}
		if err := os.Symlink(target, dst); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("copy the link %s: %w", path, err)
		}
		return nil
	case info.IsDir():
		if err := os.MkdirAll(dst, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dst, err)
		}
	default:
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: a mount point in the helper's own new root.
		if err != nil {
			return fmt.Errorf("create %s: %w", dst, err)
		}
		_ = f.Close()
	}
	if err := syscall.Mount(path, dst, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind %s: %w", path, err)
	}
	flags := syscall.MS_REMOUNT | syscall.MS_BIND | syscall.MS_NOSUID | lockedFlags(dst)
	if readOnly {
		flags |= syscall.MS_RDONLY
	}
	if info.Mode()&os.ModeDevice == 0 {
		flags |= syscall.MS_NODEV
	}
	if err := syscall.Mount("", dst, "", flags, ""); err != nil {
		return fmt.Errorf("restrict %s: %w", path, err)
	}
	return nil
}

// lockedFlags are the flags of the mount at path that a user namespace may
// not clear when it remounts a bind of it, as mount flags.
func lockedFlags(path string) uintptr {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0
	}
	var out uintptr
	for _, m := range []struct {
		st int64
		ms uintptr
	}{
		{0x0002, syscall.MS_NOSUID},
		{0x0004, syscall.MS_NODEV},
		{0x0008, syscall.MS_NOEXEC},
		{0x0400, syscall.MS_NOATIME},
		{0x0800, syscall.MS_NODIRATIME},
		{0x1000, syscall.MS_RELATIME},
		{0x0001, syscall.MS_RDONLY},
	} {
		if st.Flags&m.st != 0 {
			out |= m.ms
		}
	}
	return out
}

// setOwnLimits sets the CPU-time and address-space limits just before the
// tool starts; the shell the tool is started through sets them again.
func setOwnLimits(spec viewSpec) error {
	if spec.CPUTime > 0 {
		if err := syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: spec.CPUTime, Max: spec.CPUTime + 1}); err != nil {
			return fmt.Errorf("CPU time: %w", err)
		}
	}
	if spec.Memory > 0 {
		if err := syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: spec.Memory, Max: spec.Memory}); err != nil {
			return fmt.Errorf("address space: %w", err)
		}
	}
	return nil
}

// Capabilities: the two the helper holds in its own user namespace, and the
// prctl(2) and capset(2) values that drop them.
const (
	capSetPCap  = 8
	capSysAdmin = 21

	prCapBSetDrop           = 24
	prCapAmbient            = 47
	prCapAmbientClearAll    = 4
	linuxCapabilityVersion3 = 0x20080522
	// maxCapabilities bounds the loop over the bounding set; the kernel
	// knows about 41 today.
	maxCapabilities = 64
)

// dropCapabilities empties every capability set of the helper — bounding,
// ambient, inheritable, permitted and effective — so the tool starts with
// none, even where the builder runs as root and the tool would otherwise
// gain them all on exec.
func dropCapabilities() error {
	for c := uintptr(0); c < maxCapabilities; c++ {
		_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prCapBSetDrop, c, 0)
		if errno == syscall.EINVAL {
			break // past the last capability the kernel knows
		}
		if errno != 0 {
			return fmt.Errorf("drop capability %d from the bounding set: %w", c, errno)
		}
	}
	if _, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prCapAmbient, prCapAmbientClearAll, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("clear the ambient capabilities: %w", errno)
	}
	hdr := struct {
		version uint32
		pid     int32
	}{version: linuxCapabilityVersion3}
	var data [2]struct{ effective, permitted, inheritable uint32 }
	if _, _, errno := syscall.RawSyscall(syscall.SYS_CAPSET, uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); errno != 0 { //nolint:gosec // G103: pointers to stack values for this one call.
		return fmt.Errorf("drop the capabilities: %w", errno)
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); errno != 0 { //nolint:gosec // G103: as above.
		return fmt.Errorf("read the capabilities back: %w", errno)
	}
	for _, d := range data {
		if d.effective|d.permitted|d.inheritable != 0 {
			return errors.New("a capability is still held after dropping them all")
		}
	}
	return nil
}

// Landlock (Linux 5.13 and later): system call numbers, which are the same
// on every architecture, and the rights a ruleset can handle.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockCreateRulesetVersion = 1
	landlockRulePathBeneath      = 1

	llExecute    = 1 << 0
	llWriteFile  = 1 << 1
	llReadFile   = 1 << 2
	llReadDir    = 1 << 3
	llRemoveDir  = 1 << 4
	llRemoveFile = 1 << 5
	llMakeChar   = 1 << 6
	llMakeDir    = 1 << 7
	llMakeReg    = 1 << 8
	llMakeSock   = 1 << 9
	llMakeFifo   = 1 << 10
	llMakeBlock  = 1 << 11
	llMakeSym    = 1 << 12
	llRefer      = 1 << 13 // ABI 2
	llTruncate   = 1 << 14 // ABI 3
	llIoctlDev   = 1 << 15 // ABI 5

	llNetBindTCP    = 1 << 0 // ABI 4
	llNetConnectTCP = 1 << 1 // ABI 4

	prSetNoNewPrivs = 38

	// oPath is O_PATH on amd64 and arm64, the architectures the builder
	// runs on; the standard library does not name it.
	oPath = 0x200000
)

// restrictWithLandlock restricts the helper, and so the tool, to the view:
// read and run in the system paths and the readable directories, everything
// in the writable ones, read and write the devices, read /proc; on a kernel
// with Landlock network rules, no TCP at all. A kernel without Landlock is an
// error: the helper never starts a tool it could not restrict.
func restrictWithLandlock(spec viewSpec) error {
	abi, _, errno := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return fmt.Errorf("the kernel offers no Landlock: %w", errno)
	}
	handled := uint64(llExecute | llWriteFile | llReadFile | llReadDir | llRemoveDir | llRemoveFile |
		llMakeChar | llMakeDir | llMakeReg | llMakeSock | llMakeFifo | llMakeBlock | llMakeSym)
	if abi >= 2 {
		handled |= llRefer
	}
	if abi >= 3 {
		handled |= llTruncate
	}
	if abi >= 5 {
		handled |= llIoctlDev
	}
	var net uint64
	if abi >= 4 {
		net = llNetBindTCP | llNetConnectTCP
	}
	attr := [2]uint64{handled, net}
	fd, _, errno := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0) //nolint:gosec // G103: a pointer to a stack value for this one call.
	if errno != 0 {
		return fmt.Errorf("create a Landlock ruleset: %w", errno)
	}
	ruleset := int(fd)
	defer func() { _ = syscall.Close(ruleset) }()

	read := uint64(llExecute | llReadFile | llReadDir)
	for _, p := range append(slices.Clone(systemPaths), spec.Readable...) {
		if err := allowBeneath(ruleset, p, read); err != nil {
			return err
		}
	}
	for _, p := range spec.Writable {
		if err := allowBeneath(ruleset, p, handled); err != nil {
			return err
		}
	}
	for _, d := range devices {
		if err := allowBeneath(ruleset, d, (llReadFile|llWriteFile|llTruncate|llIoctlDev)&handled); err != nil {
			return err
		}
	}
	if err := allowBeneath(ruleset, "/proc", llReadFile|llReadDir); err != nil {
		return err
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("set no_new_privs: %w", errno)
	}
	if _, _, errno := syscall.Syscall(sysLandlockRestrictSelf, uintptr(ruleset), 0, 0); errno != 0 {
		return fmt.Errorf("restrict to the ruleset: %w", errno)
	}
	return nil
}

// allowBeneath grants access below path, which is resolved inside the view;
// a path the view does not have is left out. Rights that apply to
// directories only are dropped for a file.
func allowBeneath(ruleset int, path string, access uint64) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) || err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look at %s: %w", path, err)
	}
	if !info.IsDir() {
		access &= llExecute | llWriteFile | llReadFile | llTruncate | llIoctlDev
	}
	fd, err := syscall.Open(path, oPath|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = syscall.Close(fd) }()
	// struct landlock_path_beneath_attr is packed: a 64-bit mask and a
	// 32-bit descriptor, 12 bytes.
	var rule [12]byte
	*(*uint64)(unsafe.Pointer(&rule[0])) = access                                                                                                               //nolint:gosec // G103: writes into the packed attribute.
	*(*int32)(unsafe.Pointer(&rule[8])) = int32(fd)                                                                                                             //nolint:gosec // G103,G115: as above; a descriptor fits an int32.
	if _, _, errno := syscall.Syscall6(sysLandlockAddRule, uintptr(ruleset), landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0); errno != 0 { //nolint:gosec // G103: as above.
		return fmt.Errorf("allow %s: %w", path, errno)
	}
	return nil
}

// helperCommand turns cmd into the command that runs it through the helper.
func helperCommand(helper string, cmd Command) (Command, error) {
	if helper == "" {
		return Command{}, errors.New("builder.LinuxSandbox: no helper to isolate the child (SandboxHelperArg)")
	}
	if len(cmd.Writable) == 0 {
		return Command{}, errors.New("builder.LinuxSandbox: a child needs a writable directory")
	}
	inner := limitedCommand(cmd)
	spec := viewSpec{
		Writable: cmd.Writable, Readable: cmd.Readable, Dir: cmd.Dir, Path: inner.Path, Args: inner.Args,
	}
	if cmd.Limits.CPUTime > 0 {
		spec.CPUTime = uint64((cmd.Limits.CPUTime + time.Second - 1) / time.Second)
	}
	spec.Memory = cmd.Limits.Memory
	raw, err := json.Marshal(spec)
	if err != nil {
		return Command{}, fmt.Errorf("builder.LinuxSandbox: %w", err)
	}
	out := cmd
	out.Path, out.Args = helper, []string{SandboxHelperArg, string(raw)}
	return out, nil
}

// helperRefused reports whether a result is the helper's refusal to start a
// tool it could not isolate.
func helperRefused(res Result) bool {
	return res.ExitCode == helperUnavailableExit && strings.Contains(string(res.Stderr), helperUnavailableMarker)
}
