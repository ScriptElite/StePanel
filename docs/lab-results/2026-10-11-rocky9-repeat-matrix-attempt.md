# Rocky Linux 9 repeated recovery-matrix attempt — 2026-10-11

This was a two-iteration full recovery-matrix run on the disposable KVM host.
It did **not** pass: iteration 1 completed, while iteration 2 stopped during
the cpmove import preceding the `terminate:ownership` boundary. Do not count
this as repeated-matrix release evidence.

## Environment and result

| Item | Value |
| --- | --- |
| Guest | Rocky Linux 9 GenericCloud, SELinux enforcing |
| Harness | `deploy/lab/local-kvm-certification.sh`, `RECOVERY_MATRIX_FULL=1`, `RECOVERY_MATRIX_REPEATS=2` |
| Tested source | `16b8225` (before the audit-lock fix described below) |
| Result | FAIL; first iteration completed, second iteration incomplete |
| Failure | cpmove import smoke's HTTP request timed out after 10 seconds (curl status 28) |

The run reached the second iteration's termination cases and passed through
`terminate:site-state`. The next cpmove import request, for the
`terminate:ownership` case, timed out. The harness correctly failed the phase
instead of recording a pass.

## Diagnosis

After rebooting the disposable guest for inspection, the shared audit lock at
`/var/lib/ste-panel/audit.jsonl.lock` was a zero-byte file with a timestamp
matching a worker restart. Panel and worker logs reported `audit lock: timeout
acquiring audit lock`. The old lock acquisition created the shared path before
writing its owner metadata; interruption in that window left an empty lock
that the stale-lock reclaimer could not identify or remove. Audit-dependent
requests then stalled until their HTTP timeout.

After verifying that this exact zero-byte lock was stale, it was removed in
the disposable VM. The cpmove import smoke then passed for a fresh unique
site. This confirms the lockout mechanism; it does not turn the failed matrix
run into a pass.

The implementation now writes owner metadata to a private temporary inode and
publishes it atomically, and reclaims legacy empty locks after a short grace
period. Unit and race tests pass locally. The patched binary was then installed
in this disposable VM: a five-second-old empty lock was seeded, both panel
services were restarted, the lock disappeared during startup, and a fresh
cpmove import smoke passed. This confirms the focused host fix, but not a
successful rerun of the full repeated matrix.

The locally built test binary and guest-installed binary had the same SHA-256:
`c9128fffcf100df1f5627aad59d957196775c562d4c348d221089d75a08d7b1e`.

## Reproduction artifacts

The original run artifacts were retained under
`/var/tmp/stepanel-kvm-repeat-fixed-16b8225/` on the audit workstation,
including `install.log`, `console.log`, and `results.tsv`. The guest was
rebooted only for read-only diagnosis and the isolated cpmove reproduction;
no production host or customer data was involved.
