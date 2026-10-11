# Real ext4 backup and file-restore ENOSPC drill — 2026-10-11

This focused test extends the disposable-host disk-exhaustion evidence. It is
not a whole-host exhaustion test, database-restore test, or power-loss test.

## Environment

| Item | Value |
| --- | --- |
| Guest | Rocky Linux 9 KVM guest used by the 2026-10-09 certification |
| Filesystem | Dedicated `/dev/vdc`, ext4, 85 MiB mounted at `/mnt/stepanel-enospc` |
| Test source | `4fe8e690` plus the test and harness changes recorded in this commit |
| Commands | `TestBackupRecoversFromRealENOSPC`; `TestRestoreFilesRecoversFromRealENOSPC` |

## Results

Both tests passed sequentially on the same ext4 filesystem.

- Backup creation encountered real `ENOSPC`, published no partial backup, and
  succeeded on retry after the filler file was removed.
- File restore encountered real `ENOSPC` during extraction, preserved the
  pre-existing live-site contents, removed its temporary staging data, and
  restored the backup successfully on retry after space was reclaimed. The
  retry used an empty test document root so this focused filesystem test does
  not depend on the host's privileged site-snapshot broker.
- The ext4 `lost+found` directory was excluded from artifact checks. Each test
  uses a private temporary directory on the dedicated filesystem so artifacts
  from one test cannot contaminate the next.

This is a focused single-run result. It does not complete the repeated
20–50-run recovery gate, prove database restore under ENOSPC, or establish
host-wide behavior when unrelated services share the exhausted filesystem.

## Reproduction

On the disposable KVM guest, mount a dedicated 48–256 MiB ext4 filesystem at
`/mnt/stepanel-enospc`, then run:

```sh
STEPANEL_ENOSPC_BACKUP_ROOT=/mnt/stepanel-enospc \
STEPANEL_ENOSPC_RESTORE_ROOT=/mnt/stepanel-enospc \
go test -run '^Test(Backup|RestoreFiles)RecoversFromRealENOSPC$' -v .
```

The full disposable VM harness runs these tests in its `enospc` phase.
