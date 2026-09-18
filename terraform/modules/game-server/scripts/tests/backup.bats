#!/usr/bin/env bats
# backup.sh runs as ExecStop during shutdown: it copies the configured backup
# paths into a tarball, uploads it as _latest, keeps a timestamped copy and
# prunes history beyond the retention count.

setup() {
  load helper
  setup_sandbox

  export GAME_NAME="valheim"
  export S3_BUCKET="bonfire-valheim-backups-eu-north-1"
  export BACKUP_RETENTION_COUNT=5
  # Nothing to prune unless a test says otherwise.
  export AWS_LS_FIXTURE="${FIXTURES_DIR}/long_term_empty.txt"

  WORLD_DIR="${BATS_TEST_TMPDIR}/worlds_local"
  export BACKUP_PATHS="$WORLD_DIR"

  LATEST_KEY="${S3_BUCKET}/valheim_backup_latest.tar.gz"
}

teardown() {
  # ${GAME_NAME:?} so an aborted setup cannot turn this into rm -rf /tmp/_backup.
  rm -rf "/tmp/${GAME_NAME:?}_backup" "/tmp/${GAME_NAME:?}_backup.tar.gz"
}

@test "uploads the world data as _latest" {
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"

  run bash "${SCRIPTS_DIR}/backup.sh"

  assert_equal 0 "$status"
  assert_object_exists "$LATEST_KEY"
  run tar -tzf "${AWS_FAKE_S3}/${LATEST_KEY}"
  assert_contains "valheim_backup/worlds_local/Pupari26.db" "$output"
}

@test "writes _latest before the timestamped history copy" {
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"

  bash "${SCRIPTS_DIR}/backup.sh"

  # A spot reclaim gives roughly two minutes, so whichever write goes second is
  # the one lost when the box is cut off mid-shutdown. The pointer must not be
  # the straggler (see 81886e3).
  assert_equal \
    "s3 cp /tmp/valheim_backup.tar.gz s3://${S3_BUCKET}/valheim_backup_latest.tar.gz" \
    "$(aws_cp_calls | sed -n 1p)"
  assert_matches \
    "s3 cp s3://${S3_BUCKET}/valheim_backup_latest.tar.gz s3://${S3_BUCKET}/valheim_backup_????-??-??T??????Z.tar.gz" \
    "$(aws_cp_calls | sed -n 2p)"
}

@test "an empty world directory is a failed backup, not an empty one" {
  # cp -r of an empty worlds_local puts a *directory* in the staging area, so a
  # guard that asks whether the staging area is empty never fires. That is how
  # eight months of empty backups reported success while live world data was
  # discarded on every idle auto-stop.
  mkdir -p "$WORLD_DIR"

  run bash "${SCRIPTS_DIR}/backup.sh"

  assert_contains "Error: No files found to backup." "$output"
  assert_equal "" "$(aws_cp_calls)"
}

@test "a missing world path is a failed backup" {
  run bash "${SCRIPTS_DIR}/backup.sh"

  assert_contains "Error: No files found to backup." "$output"
  assert_equal "" "$(aws_cp_calls)"
}

@test "backs up world data buried in a subdirectory" {
  mkdir -p "${WORLD_DIR}/backups"
  echo "world data" > "${WORLD_DIR}/backups/Pupari26.db.old"

  run bash "${SCRIPTS_DIR}/backup.sh"

  assert_object_exists "$LATEST_KEY"
  run tar -tzf "${AWS_FAKE_S3}/${LATEST_KEY}"
  assert_contains "valheim_backup/worlds_local/backups/Pupari26.db.old" "$output"
}

@test "archives every path in BACKUP_PATHS" {
  # factorio backs up a directory and a file; the systemd unit joins them with a
  # space and the script splits them back apart.
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"
  echo "settings" > "${BATS_TEST_TMPDIR}/server-settings.json"
  export BACKUP_PATHS="${WORLD_DIR} ${BATS_TEST_TMPDIR}/server-settings.json"

  run bash "${SCRIPTS_DIR}/backup.sh"

  run tar -tzf "${AWS_FAKE_S3}/${LATEST_KEY}"
  assert_contains "valheim_backup/worlds_local/Pupari26.db" "$output"
  assert_contains "valheim_backup/server-settings.json" "$output"
}

@test "prunes the oldest timestamped backups past the retention count" {
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"
  export AWS_LS_FIXTURE="${FIXTURES_DIR}/short_term_listing.txt"
  export BACKUP_RETENTION_COUNT=3

  run bash "${SCRIPTS_DIR}/backup.sh"

  # The fixture holds five timestamped backups plus _latest; the two oldest go.
  removed="$(aws_rm_calls)"
  assert_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T191638Z.tar.gz" "$removed"
  assert_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T211621Z.tar.gz" "$removed"
  refute_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T212950Z.tar.gz" "$removed"
  refute_contains "valheim_backup_latest.tar.gz" "$removed"
}

@test "prunes by backup age, not by when the object was last written" {
  # A manual re-copy (re-tagging, restoring history by hand) gives an old backup
  # a fresh LastModified. Sorting the listing by its date column would then rank
  # that backup newest and prune a newer one in its place.
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"
  export AWS_LS_FIXTURE="${FIXTURES_DIR}/short_term_listing_recopied.txt"
  export BACKUP_RETENTION_COUNT=3

  run bash "${SCRIPTS_DIR}/backup.sh"

  removed="$(aws_rm_calls)"
  assert_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T191638Z.tar.gz" "$removed"
  assert_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T211621Z.tar.gz" "$removed"
  refute_contains "s3 rm s3://${S3_BUCKET}/valheim_backup_2026-09-16T212950Z.tar.gz" "$removed"
}

@test "leaves the pointer in S3 alone when the archive cannot be written" {
  # _latest is the only key restore.sh reads, so a half-written tarball must
  # never reach it. tar runs on a spot box during shutdown, against a /tmp that
  # already holds a ~50 MB copy of the world.
  mkdir -p "$WORLD_DIR"
  echo "world data" > "${WORLD_DIR}/Pupari26.db"
  export TAR_FAIL=1

  run bash "${SCRIPTS_DIR}/backup.sh"

  assert_contains "failed to create backup archive" "$output"
  assert_equal "" "$(aws_cp_calls)"
}
