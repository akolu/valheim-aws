#!/usr/bin/env bats
# restore.sh runs as ExecStartPre: when the data directory is empty it pulls
# _latest from the short-term bucket, and failing that the newest archive from
# the long-term bucket written by `bonfire retire`.

setup() {
  load helper
  setup_sandbox

  export GAME_NAME="valheim"
  export S3_BUCKET="bonfire-valheim-backups-eu-north-1"
  export DATA_PATH="${BATS_TEST_TMPDIR}/data"

  LT_BUCKET="valheim-long-term-backups"
  # The key the fixtures should resolve to: newest season, the _latest pointer.
  NEWEST_ARCHIVE="2026-09-09T160915Z/valheim_backup_latest.tar.gz"

  TARBALL="${BATS_TEST_TMPDIR}/backup.tar.gz"
  make_backup_tarball "$GAME_NAME" "$TARBALL"
  LEGACY_TARBALL="${BATS_TEST_TMPDIR}/legacy.tar.gz"
  make_legacy_tarball "$LEGACY_TARBALL"
}

teardown() {
  # ${GAME_NAME:?} so an aborted setup cannot turn this into rm -rf /tmp/_backup.
  rm -rf "/tmp/${GAME_NAME:?}_backup" "/tmp/${GAME_NAME:?}_backup.tar.gz"
  # A legacy-layout archive unpacks its own top-level directory into /tmp.
  rm -rf "/tmp/worlds_local"
}

@test "restores _latest from the short-term bucket" {
  put_object "${S3_BUCKET}/valheim_backup_latest.tar.gz" "$TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_equal 0 "$status"
  assert_equal "world data" "$(cat "${DATA_PATH}/worlds_local/Pupari26.db")"
}

@test "leaves an existing world in place" {
  mkdir -p "${DATA_PATH}/worlds_local"
  echo "live world" > "${DATA_PATH}/worlds_local/Pupari26.db"
  put_object "${S3_BUCKET}/valheim_backup_latest.tar.gz" "$TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_contains "Existing data found" "$output"
  assert_equal "live world" "$(cat "${DATA_PATH}/worlds_local/Pupari26.db")"
  assert_equal "" "$(aws_calls)"
}

@test "falls back to the newest long-term archive, not a legacy root-level key" {
  # Keys sort lexicographically and 'v' > '2', so an un-prefixed legacy key at
  # the bucket root beats every timestamped one and the oldest archive gets
  # restored (fixed in e0d5b6d).
  export AWS_LS_RECURSIVE_FIXTURE="${FIXTURES_DIR}/long_term_multi_season_with_legacy.txt"
  put_object "${LT_BUCKET}/${NEWEST_ARCHIVE}" "$TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  # The stub replays its fixture for whichever bucket it is asked about, so pin
  # the bucket name the script derives as well as the key it picks out of it.
  assert_contains "s3 ls s3://${LT_BUCKET}/ --recursive" "$(aws_calls)"
  assert_equal \
    "s3 cp s3://${LT_BUCKET}/${NEWEST_ARCHIVE} /tmp/valheim_backup.tar.gz" \
    "$(aws_cp_calls | tail -1)"
  assert_equal "world data" "$(cat "${DATA_PATH}/worlds_local/Pupari26.db")"
}

@test "picks the newest season when several are archived" {
  export AWS_LS_RECURSIVE_FIXTURE="${FIXTURES_DIR}/long_term_multi_season.txt"
  put_object "${LT_BUCKET}/${NEWEST_ARCHIVE}" "$TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_equal \
    "s3 cp s3://${LT_BUCKET}/${NEWEST_ARCHIVE} /tmp/valheim_backup.tar.gz" \
    "$(aws_cp_calls | tail -1)"
}

@test "restores nothing when only legacy root-level keys remain" {
  # Legacy archives predate the worlds_local-scoped backup_paths layout, so they
  # would restore to the wrong path. Better to start a fresh world than to
  # silently lay down a broken one.
  export AWS_LS_RECURSIVE_FIXTURE="${FIXTURES_DIR}/long_term_legacy_only.txt"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_contains "No archive found in long-term bucket" "$output"
  assert_equal "" "$(ls -A "$DATA_PATH")"
}

@test "restores nothing from an empty long-term bucket" {
  export AWS_LS_RECURSIVE_FIXTURE="${FIXTURES_DIR}/long_term_empty.txt"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_contains "No archive found in long-term bucket" "$output"
  assert_equal "" "$(ls -A "$DATA_PATH")"
}

@test "fails the unit when the short-term archive unpacks nothing" {
  # An archive whose top-level directory is not <game>_backup unpacks somewhere
  # the copy never looks. Reporting success there is the expensive failure: the
  # game starts a fresh world, and the next stop backs that world up over
  # _latest, taking the real save with it.
  put_object "${S3_BUCKET}/valheim_backup_latest.tar.gz" "$LEGACY_TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_equal 1 "$status"
  assert_contains "unpacked no files" "$output"
  assert_equal "" "$(ls -A "$DATA_PATH")"
}

@test "fails the unit when the archive unpacks only partway" {
  # A cut-short archive still leaves files in the staging directory, which is
  # enough to satisfy the "unpacked no files" guard: the unit would start the
  # game on half a world, and the next stop would back that half up over
  # _latest. backup.sh already refuses to upload a truncated archive; this is
  # the same failure on the way back in.
  TRUNCATED_TARBALL="${BATS_TEST_TMPDIR}/truncated.tar.gz"
  make_truncated_tarball "$GAME_NAME" "$TRUNCATED_TARBALL"
  put_object "${S3_BUCKET}/valheim_backup_latest.tar.gz" "$TRUNCATED_TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_equal 1 "$status"
  assert_contains "failed to unpack" "$output"
  # Nothing half-restored left behind: the next boot retries from an empty
  # directory rather than deciding it already has a world.
  assert_equal "" "$(ls -A "$DATA_PATH")"
}

@test "fails the unit when the long-term archive unpacks nothing" {
  export AWS_LS_RECURSIVE_FIXTURE="${FIXTURES_DIR}/long_term_multi_season.txt"
  put_object "${LT_BUCKET}/${NEWEST_ARCHIVE}" "$LEGACY_TARBALL"

  run bash "${SCRIPTS_DIR}/restore.sh"

  assert_equal 1 "$status"
  assert_contains "unpacked no files" "$output"
  assert_equal "" "$(ls -A "$DATA_PATH")"
}
