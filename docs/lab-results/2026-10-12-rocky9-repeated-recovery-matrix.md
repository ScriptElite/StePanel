# Rocky Linux 9 repeated recovery matrix — 2026-10-12

The disposable Rocky Linux 9 KVM guest completed two consecutive full
recovery-matrix iterations after the earlier failed attempt recorded in the
[2026-10-11 report](./2026-10-11-rocky9-repeat-matrix-attempt.md). Both
iterations exited successfully; the harness ended with
`recovery matrix passed (2 iteration(s), base prefix prodgate-20261012)`.
This is additional evidence, not production certification.

## Environment and result

| Item | Value |
| --- | --- |
| Guest | Rocky Linux 9 GenericCloud, SELinux enforcing, existing disposable KVM guest from the 2026-10-12 certification run |
| Harness | `deploy/lab/recovery-matrix-smoke.sh`, `RECOVERY_MATRIX_FULL=1`, `RECOVERY_MATRIX_REPEATS=2`, `STEPANEL_UNSAFE_LAB=1` |
| Harness source | `7d0754f4bd0e617083fbbe9d577e97904efaa71e` |
| Installed application | The same application binary retained from the 2026-10-12 certification guest; runner/integration fixes were retested separately as recorded in [the certification report](./2026-10-12-rocky9-kvm-certification.md) |
| Isolation | Unique test names derived from `prodgate-20261012`; no production host or customer data |
| Result | PASS — two consecutive full-matrix iterations, no harness-reported failures |
| Post-run services | `stepanel.service`, `stepanel-worker.service`, and `stepanel-root-broker.service` active |

The full mode exercised cpmove activation, all four backup journal boundaries,
the applicable database and file-restore boundaries, all nine site-termination
journal boundaries, and both account-suspension boundaries. The per-iteration
default matrix also exercised the Git deployment activation interruption. Each
worker or panel kill was followed by a durable-job recovery check. The guest
was responsive and ready after the harness completed.

## Limits and observations

- Two iterations are not the documented 20–50 repetitions; the repeated-run
  gate remains open. The failed 2026-10-11 run remains part of the evidence and
  is not reclassified as a pass.
- This run did not kill the root broker, simulate physical host power loss, or
  inject control-plane SQLite failure. Real ENOSPC evidence remains limited to
  backup and file restore from the separate certification runs; publication,
  import, and database-restore ENOSPC remain untested.
- The guest was a persistent test VM, not a fresh installation for each
  iteration. A quotaon systemd unit was already failed on the guest even though
  `/var/www` was mounted with user quotas enabled and the three StePanel units
  were active after the run.
- The matrix leaves one disposable cpmove fixture site from the initial
  activation-boundary case; it contains only the harness's marker file. No
  customer content was present.
- Application-level WordPress restore proof and actual external HTTP checks
  were not part of this matrix.

## Reproduction

The run used the existing guest under
`/var/tmp/stepanel-kvm-cert-555da18-current/` on the audit workstation and
executed:

```sh
sudo env RECOVERY_MATRIX_FULL=1 RECOVERY_MATRIX_REPEATS=2 \
  RECOVERY_MATRIX_PREFIX=prodgate-20261012 STEPANEL_UNSAFE_LAB=1 \
  bash /work/deploy/lab/recovery-matrix-smoke.sh
```
