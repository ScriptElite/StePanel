# Backup listing regression and focused recovery retest — 2026-10-11

## Finding

The full KVM recovery matrix stalled while listing backups for one site. On a
Rocky Linux 9.8 guest with 65 retained backup directories, `GET /api/backups`
logged a response time of 89,276 ms; the recovery script's 10-second request
deadline then expired. The panel and worker remained active. The endpoint was
verifying every archive under the backup root before filtering by site.

## Environment and change

| Item | Value |
| --- | --- |
| Guest | Rocky Linux 9.8, KVM; 3 GiB RAM, 4 vCPUs |
| Host kernel / QEMU | 7.0.0-38-generic / 10.2.1 |
| Initial matrix source | `7481167506f284b41556c832b9400803c9b15df3` |
| Listing fix | `5605c74ea1abd27790b5b5b87ebc404b81264836` |
| API listing count before fix | 65 backup directories |

The listing now reads bounded manifests, filters candidates by site, sorts
them, applies the requested limit, and verifies archive contents only for
results that can appear on the page. The regression test asserts that other
sites' and older out-of-page archives are not verified.

## Results

The focused backup/restore/termination smoke was rerun on the same guest after
building and installing the panel binary from `5605c74`. The worker and root
broker remained from the initial `7481167` installation; this isolates the
panel-side listing change. The smoke passed all injected worker-kill stages:

- Backup creation at `backup:archive`.
- File restore at `restore:activate`.
- Database restore at `restore:database`.
- Site termination at `terminate:services`.

The listing endpoint then returned in 14.018 ms and 17.894 ms in the host
journal, instead of 89,276 ms. These focused results establish the specific
regression fix and continuation of the selected recovery workflow; they do not
complete the full interruption matrix.

The requested two-iteration full matrix run against source `7481167` did not
finish: it stopped during the first full iteration at the slow backup-listing
request. It is therefore a failed/incomplete run, not a passing repetition.
A clean current-source matrix rerun and the documented 20–50-run release soak
remain required. This QEMU guest-kill test also does not establish physical
host power-loss behavior.
