# Rocky Linux 9 KVM certification — 2026-10-12

This run exercised the current `555da18` application and installer on an
SELinux-enforcing Rocky Linux 9 guest. It is additional single-run evidence,
not production certification.

## Environment

| Item | Value |
| --- | --- |
| Harness | `deploy/lab/local-kvm-certification.sh` |
| Guest | Rocky Linux 9 GenericCloud, SHA-256 `92c206cc6f790c61583247eefe87890f8828420662c17cacf247cec78ab4eec8` |
| Guest kernel | `5.14.0-687.10.1.el9_8.0.1.x86_64`, SELinux enforcing |
| Resources | 4 vCPU, 3 GiB RAM, quota-enabled 3 GiB ext4 `/var/www`, dedicated 96 MiB ENOSPC filesystem |
| Host | QEMU 10.2.1/KVM, Linux `7.0.0-38-generic` |
| Source commit | `555da18a8b905dcd1db94533d5792586e4b6e34c`, clean when the VM was built |
| Run interval | 2026-10-12 05:25–08:30 UTC |

## Results

| Phase | Result | Evidence and limits |
| --- | --- | --- |
| Boot and quota setup | PASS | `/var/www` mounted ext4 with `quota,usrquota` |
| Install and full recovery matrix | PASS | 10,636 s; 108 smoke/matrix passes; one full matrix iteration covering cpmove, backup, restore, termination, and suspension checkpoints |
| Rootless runner (initial full run) | FAIL | SELinux denied Podman's `lsetxattr` on the root-owned temporary build script (`operation not permitted`) |
| Idle abrupt guest loss | PASS | QEMU `SIGKILL`; systemd services and `/readyz` recovered in 24 s including guest boot |
| Abrupt guest loss during backup | PASS | Interrupted job reconciled to completed; listed backup verified; no staging leftovers; a new backup completed and verified (244 s) |
| Real ENOSPC | PASS | Backup and file-restore ENOSPC test passed on the dedicated ext4 filesystem (31 s) |
| Runner fix retest | PASS | After changing the staged script to site-owned/read-only, the same persistent guest passed the two-build/failed-build runner test and the Git checkout → build → activation release-pipeline test (31 s) |

The initial runner failure was reproducible: `/run/stepanel-runner-input…/build.sh`
was root-owned, while rootless Podman needed to apply a private SELinux MCS
label to that bind-mounted file. The targeted retest used the modified
`deploy/integrations/stepanel-runnerctl` installed into the disposable guest;
the panel and worker binaries remained those built from `555da18`. The source
change, regression assertion, and changelog entry are in the follow-up commit.
The initial full harness results retain the runner FAIL row; the independent
runner-only harness retest records PASS. Together these establish the failure
and validate the fix, but do not constitute a clean all-green rerun of every
phase on the modified source tree.

The ENOSPC result covers the backup and file-restore test cases only. QEMU
process termination is not physical host power loss. The full journal matrix
ran once, not the required 20–50 repetitions. Application-level WordPress
proof on a freshly provisioned host, MariaDB-loss injection during database
restore, physical power-loss recovery, and broader workload/capacity evidence
remain open.

## Reproduction artifacts

- Full run directory: `/var/tmp/stepanel-kvm-cert-555da18-current/`
- Full-run summary: `results.tsv`; guest install log: `install.log`
- Runner fix retest: `/var/tmp/stepanel-kvm-runner-fix-555da18/`
- Hosted Ubuntu quota-install run `38206831111`: [GitHub Actions run](https://github.com/cyberducttape/StePanel/actions/runs/38206831111), completed successfully.
