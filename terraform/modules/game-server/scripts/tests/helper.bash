# Shared setup for the backup/restore bats suites.
# shellcheck shell=bash
# SCRIPTS_DIR and FIXTURES_DIR are read by the .bats files that load this.
# shellcheck disable=SC2034

SCRIPTS_DIR="$(cd "${BATS_TEST_DIRNAME}/.." && pwd)"
FIXTURES_DIR="${BATS_TEST_DIRNAME}/fixtures"

# Puts the aws/chown stubs first on PATH and gives the test an empty fake
# bucket store plus an empty call log. Call this from every setup().
setup_sandbox() {
  PATH="${BATS_TEST_DIRNAME}/stubs:$PATH"
  export PATH
  export AWS_CALL_LOG="${BATS_TEST_TMPDIR}/aws-calls.log"
  export AWS_FAKE_S3="${BATS_TEST_TMPDIR}/s3"
  : > "$AWS_CALL_LOG"
  mkdir -p "$AWS_FAKE_S3"
}

aws_calls() {
  cat "$AWS_CALL_LOG"
}

# Writes a file into the fake object store, e.g. put_object bucket/key file
put_object() {
  mkdir -p "$(dirname "${AWS_FAKE_S3}/$1")"
  cp "$2" "${AWS_FAKE_S3}/$1"
}

assert_object_exists() {
  if [ ! -f "${AWS_FAKE_S3}/$1" ]; then
    printf 'expected object in the bucket: %s\nbucket holds:\n%s\n' "$1" "$(cd "$AWS_FAKE_S3" && find . -type f)" >&2
    return 1
  fi
}

# Builds a tarball shaped like one backup.sh produces: a <game>_backup
# directory holding the copied backup paths.
make_backup_tarball() {
  local game="$1" dest="$2"
  local stage="${BATS_TEST_TMPDIR}/stage"
  rm -rf "$stage"
  mkdir -p "${stage}/${game}_backup/worlds_local"
  echo "world data" > "${stage}/${game}_backup/worlds_local/Pupari26.db"
  echo "world meta" > "${stage}/${game}_backup/worlds_local/Pupari26.fwl"
  tar -czf "$dest" -C "$stage" "${game}_backup"
}

aws_cp_calls() {
  grep '^s3 cp' "$AWS_CALL_LOG" || true
}

aws_rm_calls() {
  grep '^s3 rm' "$AWS_CALL_LOG" || true
}

# Assertions.
#
# Deliberately functions rather than bare `[[ ... ]]`: bats does not fail a test
# when a `[[ ... ]]` keyword returns non-zero mid-body, so inline conditions
# silently pass. A function returning 1 does fail the test, and reports what it
# expected while it is at it.

assert_contains() {
  case "$2" in
    *"$1"*) ;;
    *) printf 'expected output to contain: %s\nactual: %s\n' "$1" "$2" >&2; return 1 ;;
  esac
}

refute_contains() {
  case "$2" in
    *"$1"*) printf 'expected output NOT to contain: %s\nactual: %s\n' "$1" "$2" >&2; return 1 ;;
  esac
}

# Glob match, for assertions where part of the string is a timestamp.
assert_matches() {
  # shellcheck disable=SC2254  # an unquoted pattern is the point
  case "$2" in
    $1) ;;
    *) printf 'expected to match: %s\nactual: %s\n' "$1" "$2" >&2; return 1 ;;
  esac
}

assert_equal() {
  if [ "$1" != "$2" ]; then
    printf 'expected: %s\nactual:   %s\n' "$1" "$2" >&2
    return 1
  fi
}

# An archive whose gzip stream stops partway, as an interrupted download or a
# /tmp that filled up during extraction leaves it. tar writes out what it has
# read before it hits the cut, so some of the world lands in the staging
# directory and the extraction still fails.
make_truncated_tarball() {
  local game="$1" dest="$2"
  local stage="${BATS_TEST_TMPDIR}/truncated_stage"
  rm -rf "$stage"
  mkdir -p "${stage}/${game}_backup/worlds_local"
  # Incompressible and well over tar's record size, so there is something on
  # both sides of the cut: files extracted, and an archive that ends early.
  head -c 1048576 /dev/urandom > "${stage}/${game}_backup/worlds_local/Pupari26.db"
  echo "world meta" > "${stage}/${game}_backup/worlds_local/Pupari26.fwl"
  tar -czf "${dest}.whole" -C "$stage" "${game}_backup"
  head -c 400000 "${dest}.whole" > "$dest"
  rm -f "${dest}.whole"
}

# An archive from before the worlds_local-scoped backup_paths layout: its
# top-level directory is not <game>_backup, so unpacking it puts nothing where
# restore.sh looks.
make_legacy_tarball() {
  local dest="$1"
  local stage="${BATS_TEST_TMPDIR}/legacy_stage"
  rm -rf "$stage"
  mkdir -p "${stage}/worlds_local"
  echo "old world" > "${stage}/worlds_local/Pupari20.db"
  tar -czf "$dest" -C "$stage" "worlds_local"
}
