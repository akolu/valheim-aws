# Listing fixtures

The restore and prune paths parse `aws s3 ls` output with `awk '{print $NF}'`, so a
hand-written fixture would only test our assumption about that format. These files are
captured from the real CLI instead.

| File | Provenance |
|---|---|
| `long_term_multi_season_with_legacy.txt` | `aws s3 ls s3://valheim-long-term-backups/ --recursive`, 2026-09-09, right after a `bonfire retire`. Holds two seasons, two `_latest` decoys and the two legacy root-level keys that caused the oldest archive to be restored (`v` sorts after `2`). |
| `long_term_multi_season.txt` | The same capture with the legacy root-level lines removed. |
| `long_term_legacy_only.txt` | Only the legacy root-level lines from that capture. |
| `long_term_empty.txt` | Empty bucket: `aws s3 ls` prints nothing and exits 0. |
| `short_term_listing.txt` | `aws s3 ls s3://bonfire-valheim-backups-eu-north-1/`, 2026-09-18. Five timestamped backups plus `_latest`, the steady state after the prune started working (#18). Note the different column layout — this listing is not `--recursive`, so keys carry no prefix. |

`short_term_listing_recopied.txt` is the one derived file: the same capture with a single
object's LastModified bumped, as happens when a backup is re-copied by hand. Key order is
left alone, since `aws s3 ls` returns keys lexicographically rather than by date.
