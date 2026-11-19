<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0106. Each build runs in a private view of the filesystem, restricted by Landlock, with no capabilities

- **Status:** Accepted (maintainer, 2026-11-19)
- **Date:** 2026-11-19
- **Requirements:** `FN-016`, `FN-019`, `SEC-054`
- **Amends:** [ADR-0063](0063-function-builds-provenance.md) (the build sandbox)

## Context and problem

ADR-0063 runs each build in new user, network, mount and PID namespaces, "with the vendored
sources read-only and a scratch directory". The mount namespace was created, but nothing was
mounted in it: a build ran as the builder's own user and saw the builder's whole filesystem.
A test child wrote a file outside its build directory (build log #164). Builds of different
organisations run side by side in one builder, so a build could read or change another
organisation's workspace while it built. Since ADR-0105 the only native code a build runs is
pinned, published crates and the compilers, so this needed a compiler exploit; but the result
would cross tenants, and the server signs the module. It also ruled out sharing compiled
dependencies between builds of one organisation (build log #161).

Two further findings came from running the sandbox as it is deployed — in a container as user
65532, with every capability dropped, `no-new-privileges` and the builder's seccomp profile:

- a child that keeps the builder's non-root user loses every capability of its new user
  namespace when it starts a program, so it cannot mount anything there;
- the kernel mounts a new `/proc` inside a user namespace only where the container's `/proc`
  is not partly hidden, and Docker and Kubernetes hide parts of it by default; rustc and Bun
  (which runs `wasm-opt`) do not run without a `/proc`.

## Decision drivers

- Security first (spec §1.4): a build reads and writes only what is its own, and sees no other
  build's files or processes. No existing control is weakened.
- Fail closed: where the machine cannot isolate a build, the builder refuses to build, as it
  already does for namespaces.
- Builds stay byte-identical (`FN-019`) and about as fast.
- No new dependency (the allowlist of dependencies.md).

## Considered options

1. **Landlock only.** The child restricts itself to the toolchain (read) and its scratch
   directory (write) before the tool starts. Works in every container as shipped, but the tools
   need `/proc`, and the container's `/proc` shows every process of the builder, including the
   command lines of other organisations' builds.
2. **A private view and Landlock** (this decision). The child gets its own root holding only
   what the build needs, a `/proc` of its own PID namespace, and Landlock on top. Needs the
   container's `/proc` unmasked and the sandbox's mounts allowed by seccomp and AppArmor.
3. **A private view with the container's `/proc`.** Hides files but not other builds'
   processes.
4. **A micro-VM per build** (Firecracker, gVisor). The strongest boundary, but another
   runtime to operate, with nested virtualisation or a different container runtime on every
   node; kept for later if the threat model calls for it.

## Decision

Chosen option: **2**.

**The helper.** Every child of the Linux sandbox starts as the builder's own executable with
the argument `__plux-sandbox-child` and a JSON specification (`Command.Writable`,
`Command.Readable`, the tool and its limits). The child is started with two capabilities as
ambient capabilities — `CAP_SYS_ADMIN` and `CAP_SETPCAP` — which it holds only in its own new
user namespace. Before the tool starts, the helper:

1. makes its mounts private and mounts a small `tmpfs` as the new root, below the build's
   scratch directory;
2. binds into it, each at its own path: read-only and `nosuid` the system directories (`/usr`,
   `/bin`, `/sbin`, `/lib*`, `/etc/alternatives` and the few `/etc` files the dynamic linker and
   the C library read) and the toolchain directories; writable, `nosuid` and `nodev`, the
   build's own scratch directory; and `/dev/null`, `/dev/zero`, `/dev/random`, `/dev/urandom`.
   A bind keeps the flags the machine locked on the source;
3. mounts a new `/proc` — the child is the first process of its own PID namespace, so it shows
   only the build's processes — and hides in it, as Docker does, the files that describe the
   whole machine (`kcore`, `keys`, `timer_list`, `sched_debug`, `acpi`, `scsi` and the rest);
4. pivots into the new root, detaches the old one and makes the root read-only;
5. sets the CPU-time and address-space limits;
6. drops every capability: the bounding, ambient, inheritable, permitted and effective sets,
   and checks they are empty — so the tool holds none, even where the builder runs as root;
7. restricts itself with Landlock (Linux 5.13 or later): read and run in the system and
   toolchain directories, everything in the scratch directory, read and write the four
   devices, read `/proc`; on kernels with Landlock network rules (ABI 4), no TCP at all;
   `no_new_privs` is set;
8. replaces itself with the tool, with `TMPDIR` in the scratch directory.

A machine that refuses any of this — no Landlock, a refused mount, a refused `/proc` — makes
the helper exit with a marked status, and the builder reports `ErrSandboxUnavailable` and
refuses to build. The Landlock ruleset handles every access right the kernel's ABI knows
(`REFER` from ABI 2, `TRUNCATE` from 3, `IOCTL_DEV` from 5).

**What the tools can no longer do.** They cannot read, list, run or change anything outside
the view or their scratch directory; cannot see other builds' processes; cannot open a TCP
connection where the kernel has Landlock network rules; and cannot change the view: Landlock
refuses `mount(2)`, `umount(2)` and `pivot_root(2)` to a restricted process and its children
before any filesystem code runs, in a nested user namespace too (a test proves it).

**Deployment.** The sandbox needs, beyond the hardened defaults:

- **seccomp** (`deploy/compose/seccomp-builder.json`, also the Kubernetes Localhost profile):
  besides `clone` with the four namespaces, `mount` with only the flags the helper uses
  (`0x245C2F`: `MS_RDONLY`, `MS_NOSUID`, `MS_NODEV`, `MS_NOEXEC`, `MS_REMOUNT`, `MS_NOATIME`,
  `MS_NODIRATIME`, `MS_BIND`, `MS_REC`, `MS_PRIVATE`, `MS_RELATIME`), `umount2` with
  `MNT_DETACH` only, and `pivot_root`. No capability is added: outside its own namespaces the
  builder cannot mount, and the tools cannot (above).
- **An unmasked `/proc`** for the builder container: Docker `systempaths=unconfined`;
  Kubernetes `procMount: Unmasked`, which requires a user-namespaced pod (`hostUsers: false`,
  Kubernetes 1.33 or later with user namespaces on the nodes). The builder runs as an
  unprivileged user with no capability, so the files Docker masks behind root-only permissions
  stay closed to it; the build's view masks them again. In Kubernetes the builder's users are
  then not the node's users at all.
- **AppArmor**, where the host has it: the runtime's default profile denies every mount, so
  `deploy/compose/apparmor-plux-builder` — Docker's default profile with `mount` and
  `pivot_root` allowed and nothing else changed — is loaded on the host and named
  (`apparmor=plux-builder`, Helm `builder.appArmorProfile`).
- **Landlock** in the kernel (5.13 or later, `landlock` among the active security modules).

The chart's lint checks that only the builder has an unmasked `/proc` and only together with
`hostUsers: false`.

## Consequences

- **Positive:** a build cannot reach another build's files or processes, the builder's files or
  anything else on the machine; the stated sandbox of ADR-0063 now holds and goes further
  (no capabilities, no TCP, no mount changes); a per-organisation build cache on the server
  becomes possible (build log #161, its own ADR). Measured: starting a tool in the sandbox takes
  9.5 ms against 0.8 ms unsandboxed; a build starts a handful of tools, and the compilers
  cargo starts run inside the same view. The sandboxed real-toolchain build is byte-identical
  to the unsandboxed one.
- **Negative:** deployments need the settings above, and a builder on a machine without them
  refuses to build (fail closed, with the reason in the log). The seccomp profile and the
  AppArmor profile now allow mount calls, which serve only the helper; the builder container's
  `/proc` is unmasked. The Kubernetes and AppArmor settings are proven by the CI walkthrough on
  kind; this repository's sandbox tests prove the rest in hardened Docker and on the host.
- **Follow-up:** the per-organisation server build cache (#161); macOS and Windows builders
  keep refusing the sandbox as before.
