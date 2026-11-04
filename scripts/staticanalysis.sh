#!/usr/bin/env bash
# staticanalysis.sh
# Run go vet + staticcheck + semgrep. Each tool gets its own report and plan.
#
# Origin: github.com/trypanic/skills go-code-quality-check/scripts/staticanalysis.sh
# at e16a751. This copy is owned by go-sdk: Taskfile, pre-commit and CI run it,
# so a fresh clone needs no skill install. Change it here, in this repository.
#
# Modes:
#   default            — staged .go files only (pre-commit)
#   QG_FULL=1          — full repo (./...)
#   QG_SKIP_VET=1      — skip go vet
#   QG_SKIP_SC=1       — skip staticcheck
#   QG_SKIP_SEMGREP=1  — skip semgrep
#   QG_SEMGREP_PACKS=0 — semgrep with .semgrep.yml only (no registry packs)
#   QG_SEMGREP_IMAGE=… — override the pinned semgrep Docker image
#   QG_VERBOSE=1       — also print tool stderr to the terminal (it is always
#                        logged to .staticanalysis/<tool>.stderr.log)
#
# Outputs (all under .staticanalysis/):
#   .staticanalysis/report.<tool>.json   findings
#   .staticanalysis/plan.<tool>.md       grouped markdown plan
#   .staticanalysis/<tool>.stderr.log    suppressed tool stderr (for debugging)
#
# Override location with QG_OUT_DIR=path.
#
# Exit codes:
#   0  clean (no ERROR-severity finding, every enabled tool ran)
#   1  ERROR-severity finding(s) in any tool
#   2  a tool failed (crashed, exited with an unexpected code, produced no
#      parsable output) or a required tool is missing. A tool that is not
#      installed is a failure unless its QG_SKIP_* knob is set; jq is always
#      required.

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "staticanalysis: jq is required (sudo apt install jq / brew install jq)." >&2
  exit 2
fi

OUT_DIR="${QG_OUT_DIR:-.staticanalysis}"
mkdir -p "${OUT_DIR}"

VET_REPORT="${OUT_DIR}/report.go-vet.json"
SC_REPORT="${OUT_DIR}/report.staticcheck.json"
SG_REPORT="${OUT_DIR}/report.semgrep.json"

VET_PLAN="${OUT_DIR}/plan.go-vet.md"
SC_PLAN="${OUT_DIR}/plan.staticcheck.md"
SG_PLAN="${OUT_DIR}/plan.semgrep.md"

VET_RAW="${OUT_DIR}/.vet-raw.txt"
SC_RAW="${OUT_DIR}/.staticcheck-raw.json"
SG_RAW="${OUT_DIR}/.semgrep-raw.json"

VET_ERR_LOG="${OUT_DIR}/go-vet.stderr.log"
SC_ERR_LOG="${OUT_DIR}/staticcheck.stderr.log"
SG_ERR_LOG="${OUT_DIR}/semgrep.stderr.log"

cleanup_scratch() {
  rm -f "${VET_RAW}" "${SC_RAW}" "${SG_RAW}"
  if declare -f stop_spinner >/dev/null; then
    stop_spinner
  fi
}
trap cleanup_scratch EXIT

VET_ERRORS=0; VET_TOTAL=0
SC_ERRORS=0; SC_TOTAL=0
SG_ERRORS=0; SG_TOTAL=0

# Pinned semgrep: the Docker image by digest, and the version that
# `task tools:install` installs for the CLI fallback (keep both in sync).
SEMGREP_VERSION="1.95.0"
SEMGREP_IMAGE="${QG_SEMGREP_IMAGE:-semgrep/semgrep:${SEMGREP_VERSION}@sha256:30e6afa99ebd8e7b4115d4904898108eb4bf77025819e9263f09cf14e6f6e549}"

# Tool failures (crash, unexpected exit code, missing tool). Any -> exit 2.
TOOL_FAILURES=()
SG_RUNNER="none"

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TTY=0; [[ -t 1 ]] && TTY=1
USE_SPINNER=0
[[ "${TTY}" == "1" && "${QG_VERBOSE:-0}" != "1" ]] && USE_SPINNER=1
TOTAL_STEPS=3
SPINNER_PID=""

# ---------- mode + targets ----------------------------------------------------

if [[ "${QG_FULL:-0}" == "1" ]]; then
  MODE="full"
  GO_PKGS=("./...")
  SEMGREP_TARGETS=(".")
  STAGED_FILES=()
else
  MODE="staged"
  mapfile -t STAGED_FILES < <(git diff --cached --name-only --diff-filter=ACMR 2>/dev/null | grep -E '\.go$' || true)
  if [[ ${#STAGED_FILES[@]} -eq 0 ]]; then
    echo "staticanalysis: no staged .go files, skipping."
    exit 0
  fi
  declare -A PKG_SET=()
  for f in "${STAGED_FILES[@]}"; do
    d="$(dirname "$f")"
    [[ "$d" == "." ]] && PKG_SET["./..."]=1 || PKG_SET["./${d}"]=1
  done
  GO_PKGS=("${!PKG_SET[@]}")
  SEMGREP_TARGETS=("${STAGED_FILES[@]}")
fi

printf "staticanalysis: mode=%s pkgs=%d files=%d → %s\n" \
  "${MODE}" "${#GO_PKGS[@]}" "${#SEMGREP_TARGETS[@]}" "${OUT_DIR}"

# ---------- progress ui -------------------------------------------------------

start_spinner() {
  local prefix="$1"
  [[ "${USE_SPINNER}" != "1" ]] && return 0
  (
    local frames='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local n=${#frames}
    local i=0
    while :; do
      printf "\r\033[K%s %s" "$prefix" "${frames:i:1}"
      i=$(( (i + 1) % n ))
      sleep 0.1
    done
  ) &
  SPINNER_PID=$!
  disown "$SPINNER_PID" 2>/dev/null || true
}

stop_spinner() {
  if [[ -n "${SPINNER_PID:-}" ]]; then
    kill "${SPINNER_PID}" 2>/dev/null || true
    wait "${SPINNER_PID}" 2>/dev/null || true
    SPINNER_PID=""
  fi
}

step_done() {
  local n="$1" name="$2" status="$3" detail="$4"
  stop_spinner
  if [[ "${TTY}" == "1" ]]; then
    printf "\r\033[K[%d/%d] %-12s %s %s\n" "$n" "$TOTAL_STEPS" "$name" "$status" "$detail"
  else
    printf "[%d/%d] %s %s %s\n" "$n" "$TOTAL_STEPS" "$name" "$status" "$detail"
  fi
}

step_start() {
  local n="$1" name="$2"
  local prefix
  prefix="$(printf "[%d/%d] %-12s running" "$n" "$TOTAL_STEPS" "$name")"
  if [[ "${USE_SPINNER}" == "1" ]]; then
    start_spinner "$prefix"
  else
    printf "%s\n" "$prefix"
  fi
}

# ---------- helpers -----------------------------------------------------------

wrap_report() {
  local tool="$1" findings_file="$2" runner="${3:-}"
  jq -n \
    --slurpfile findings "${findings_file}" \
    --arg tool "${tool}" \
    --arg mode "${MODE}" \
    --arg ts "${TS}" \
    --arg runner "${runner}" \
    '
    ($findings[0] // []) as $all
    | {
        tool: $tool, generated_at: $ts, mode: $mode,
        runner: (if $runner == "" then null else $runner end),
        summary: {
          total:    ($all | length),
          errors:   ($all | map(select(.severity == "ERROR"))   | length),
          warnings: ($all | map(select(.severity == "WARNING")) | length),
          info:     ($all | map(select(.severity == "INFO"))    | length)
        },
        findings: $all
      }
    '
}

write_plan() {
  local tool="$1" report="$2" plan="$3"
  local total errors warns infos
  total=$(jq '.summary.total'    "${report}")
  errors=$(jq '.summary.errors'   "${report}")
  warns=$(jq  '.summary.warnings' "${report}")
  infos=$(jq  '.summary.info'     "${report}")
  {
    echo "# ${tool} plan (${MODE})"
    echo
    echo "- Generated: $(jq -r '.generated_at' "${report}")"
    if [[ "$(jq -r '.runner // ""' "${report}")" != "" ]]; then
      echo "- Runner: $(jq -r '.runner' "${report}")"
    fi
    echo "- Findings: **${total}** (errors=${errors}, warnings=${warns}, info=${infos})"
    echo "- Report: \`${report}\`"
    echo
    if [[ "${total}" -gt 0 ]]; then
      echo "## Fixes"
      echo
      jq -r '
        .findings
        | group_by(.path)
        | .[]
        | "### \(.[0].path)\n" +
          ( map("- **[\(.severity)]** L\(.line) — `\(.check_id)` — \(.message | gsub("\n";" "))")
            | join("\n") )
          + "\n"
      ' "${report}"
    else
      echo "_No findings._"
    fi
  } > "${plan}"
}

# Tool stderr always goes to its log file (go vet writes its diagnostics
# there, so the log is also its raw report). QG_VERBOSE=1 echoes it afterwards.
show_stderr() {
  local log="$1"
  if [[ "${QG_VERBOSE:-0}" == "1" && -s "${log}" ]]; then
    cat "${log}" >&2
  fi
}

# tool_failed STEP NAME REASON [LOG] records the failure and prints the tail
# of the tool's stderr so the cause is visible without opening the log.
tool_failed() {
  local n="$1" name="$2" reason="$3" log="${4:-}"
  TOOL_FAILURES+=("${name}: ${reason}")
  step_done "$n" "$name" "$FAIL_MARK" "TOOL FAILURE: ${reason}"
  if [[ -n "${log}" && -s "${log}" && "${QG_VERBOSE:-0}" != "1" ]]; then
    echo "  last lines of ${log}:" >&2
    tail -n 5 "${log}" | sed 's/^/    /' >&2
  fi
}

# Compose a one-line summary detail.
detail_str() {
  local total="$1" errs="$2" rc="$3"
  printf "(%d findings, %d errors, rc=%d)" "$total" "$errs" "$rc"
}

# Emoji-free status markers for non-UTF terminals: use plain text.
OK_MARK="✓"
FAIL_MARK="✗"
SKIP_MARK="·"

# ---------- go vet ------------------------------------------------------------

run_vet() {
  if [[ "${QG_SKIP_VET:-0}" == "1" ]]; then
    step_done 1 "go-vet" "$SKIP_MARK" "skipped"
    return 0
  fi
  if ! command -v go >/dev/null 2>&1; then
    tool_failed 1 "go-vet" "go is not installed (QG_SKIP_VET=1 to skip)"
    return 0
  fi
  step_start 1 "go-vet"

  set +e
  GOCACHE="${GOCACHE:-/tmp/go-build-cache}" go vet "${GO_PKGS[@]}" >"${VET_RAW}" 2>"${VET_ERR_LOG}"
  local rc=$?
  set -e
  show_stderr "${VET_ERR_LOG}"
  # vet emits diagnostics on stderr; treat captured log as the raw report too.
  [[ -s "${VET_ERR_LOG}" && ! -s "${VET_RAW}" ]] && cp "${VET_ERR_LOG}" "${VET_RAW}"

  # Parse "file:line[:col]: message" into TSV. Type-check errors carry a
  # "vet: " prefix and may continue on tab-indented lines ("\thave ()",
  # "\twant (int)"), which are appended to that diagnostic. Every other
  # non-blank, non-"#" line (e.g. a vet panic and its stack) is unparsable
  # and kept apart; a tab-indented line counts as a continuation only right
  # after a diagnostic.
  local findings_tmp vet_tsv vet_unparsed
  findings_tmp="$(mktemp)"; vet_tsv="$(mktemp)"; vet_unparsed="$(mktemp)"
  awk -v unparsed="${vet_unparsed}" '
    function flush() { if (pending != "") print pending; pending = "" }
    /^#/ || /^[[:space:]]*$/ { flush(); next }
    /^\t/ {
      if (pending != "") { c = $0; sub(/^[[:space:]]+/, "", c); pending = pending " " c; next }
      print > unparsed; next
    }
    {
      flush()
      text = $0
      sub(/^vet: /, "", text)
      n = index(text, ":")
      n2 = (n > 0) ? index(substr(text, n+1), ":") : 0
      line = (n2 > 0) ? substr(text, n+1, n2-1) : ""
      if (n <= 1 || line !~ /^[0-9]+$/) { print > unparsed; next }
      file = substr(text, 1, n-1)
      sub(/^\.\//, "", file)
      rest2 = substr(text, n+n2+1)
      n3 = index(rest2, ":")
      col = "0"
      msg = rest2
      if (n3 > 0 && substr(rest2,1,n3-1) ~ /^[0-9]+$/) {
        col = substr(rest2, 1, n3-1)
        msg = substr(rest2, n3+1)
      }
      sub(/^[[:space:]]+/, "", msg)
      gsub(/\t/, " ", msg)
      pending = file "\t" line "\t" col "\t" msg
    }
    END { flush() }
  ' "${VET_RAW}" > "${vet_tsv}"
  # jq builds the JSON, so quotes, backslashes and tabs in messages are safe.
  jq -R -s 'split("\n") | map(select(length > 0) | split("\t") |
    {severity: "ERROR", path: .[0], line: (.[1] | tonumber),
     column: (.[2] | tonumber), check_id: "go-vet",
     message: (.[3:] | join("\t"))})' "${vet_tsv}" > "${findings_tmp}"

  wrap_report "go-vet" "${findings_tmp}" > "${VET_REPORT}"
  rm -f "${findings_tmp}" "${vet_tsv}"
  write_plan "go-vet" "${VET_REPORT}" "${VET_PLAN}"

  VET_ERRORS=$(jq '.summary.errors' "${VET_REPORT}")
  VET_TOTAL=$(jq '.summary.total'  "${VET_REPORT}")
  # go vet exits 1 for diagnostics and for build/load errors. A non-zero exit
  # without a single parsed diagnostic means it did not analyse anything.
  # Unparsable output on a failing run (a panic, a crash message) means the
  # analysis did not complete, even if some diagnostics were parsed.
  local unparsed_n
  unparsed_n=$(wc -l < "${vet_unparsed}" | tr -d ' ')
  rm -f "${vet_unparsed}"
  if [[ "${rc}" -ne 0 && "${VET_TOTAL}" -eq 0 ]]; then
    tool_failed 1 "go-vet" "exit ${rc} without parsable diagnostics" "${VET_ERR_LOG}"
    return 0
  fi
  if [[ "${rc}" -ne 0 && "${unparsed_n}" -gt 0 ]]; then
    tool_failed 1 "go-vet" "exit ${rc} with ${unparsed_n} unparsable output line(s)" "${VET_ERR_LOG}"
    return 0
  fi
  local mark="$OK_MARK"; [[ "${VET_ERRORS}" -gt 0 ]] && mark="$FAIL_MARK"
  step_done 1 "go-vet" "$mark" "$(detail_str "$VET_TOTAL" "$VET_ERRORS" "$rc")"
}

# ---------- staticcheck -------------------------------------------------------

run_staticcheck() {
  if [[ "${QG_SKIP_SC:-0}" == "1" ]]; then
    step_done 2 "staticcheck" "$SKIP_MARK" "skipped"
    return 0
  fi
  if ! command -v staticcheck >/dev/null 2>&1; then
    tool_failed 2 "staticcheck" "not installed (proto install; QG_SKIP_SC=1 to skip)"
    return 0
  fi
  step_start 2 "staticcheck"

  set +e
  XDG_CACHE_HOME="${XDG_CACHE_HOME:-/tmp/staticanalysis-cache}" staticcheck -f json "${GO_PKGS[@]}" >"${SC_RAW}" 2>"${SC_ERR_LOG}"
  local rc=$?
  set -e
  show_stderr "${SC_ERR_LOG}"

  # staticcheck exits 0 when clean and 1 when it reports problems (as JSON
  # lines on stdout). Anything else, or 1 with no output, is a crash.
  if [[ "${rc}" -ne 0 && "${rc}" -ne 1 ]]; then
    tool_failed 2 "staticcheck" "unexpected exit ${rc}" "${SC_ERR_LOG}"
    return 0
  fi
  if [[ "${rc}" -eq 1 && ! -s "${SC_RAW}" ]]; then
    tool_failed 2 "staticcheck" "exit 1 without findings on stdout" "${SC_ERR_LOG}"
    return 0
  fi
  if [[ -s "${SC_RAW}" ]] && ! jq -s '.' "${SC_RAW}" >/dev/null 2>&1; then
    tool_failed 2 "staticcheck" "output is not valid JSON" "${SC_ERR_LOG}"
    return 0
  fi

  local findings_tmp
  findings_tmp="$(mktemp)"
  if [[ -s "${SC_RAW}" ]]; then
    jq -s '
      [ .[] | {
          severity: (.severity // "ERROR" | ascii_upcase),
          path: .location.file,
          line: (.location.line // 0),
          column: (.location.column // 0),
          check_id: .code,
          message: .message
        }
      ]
    ' "${SC_RAW}" > "${findings_tmp}"
  else
    echo "[]" > "${findings_tmp}"
  fi

  wrap_report "staticcheck" "${findings_tmp}" > "${SC_REPORT}"
  rm -f "${findings_tmp}"
  write_plan "staticcheck" "${SC_REPORT}" "${SC_PLAN}"

  SC_ERRORS=$(jq '.summary.errors' "${SC_REPORT}")
  SC_TOTAL=$(jq '.summary.total'  "${SC_REPORT}")
  local mark="$OK_MARK"; [[ "${SC_ERRORS}" -gt 0 ]] && mark="$FAIL_MARK"
  step_done 2 "staticcheck" "$mark" "$(detail_str "$SC_TOTAL" "$SC_ERRORS" "$rc")"
}

# ---------- semgrep -----------------------------------------------------------

run_semgrep() {
  if [[ "${QG_SKIP_SEMGREP:-0}" == "1" ]]; then
    step_done 3 "semgrep" "$SKIP_MARK" "skipped"
    return 0
  fi

  local have_docker=0 have_cli=0
  docker info >/dev/null 2>&1 && have_docker=1
  command -v semgrep >/dev/null 2>&1 && have_cli=1
  if [[ "${have_docker}" == "0" && "${have_cli}" == "0" ]]; then
    tool_failed 3 "semgrep" "neither Docker nor the semgrep CLI is available (task tools:install; QG_SKIP_SEMGREP=1 to skip)"
    return 0
  fi
  step_start 3 "semgrep"

  : >"${SG_ERR_LOG}"
  rm -f "${SG_REPORT}" "${SG_RAW}"

  # Same rules on both paths so the result does not depend on which one ran.
  # The community packs come from the Semgrep registry at run time and cannot
  # be pinned; QG_SEMGREP_PACKS=0 restricts the scan to .semgrep.yml (offline,
  # fully reproducible).
  local configs=(--config=.semgrep.yml) config_desc=".semgrep.yml"
  if [[ "${QG_SEMGREP_PACKS:-1}" != "0" ]]; then
    configs+=(--config=p/golang --config=p/gosec --config=p/secrets)
    config_desc+=" p/golang p/gosec p/secrets"
  fi

  # semgrep (without --error) exits 0 whether or not it finds something;
  # any other exit code, or no JSON report, is a failed run.
  local rc=0 attempts=""
  set +e
  if [[ "${have_docker}" == "1" ]]; then
    echo "--- semgrep via docker (${SEMGREP_IMAGE}; ${config_desc})" >>"${SG_ERR_LOG}"
    # The report goes to stdout so QG_OUT_DIR may live outside the mount.
    docker run --rm -v "${PWD}:/src" -w /src "${SEMGREP_IMAGE}" semgrep scan \
      "${configs[@]}" \
      --json \
      --quiet \
      --metrics=off \
      --disable-version-check \
      "${SEMGREP_TARGETS[@]}" >"${SG_RAW}" 2>>"${SG_ERR_LOG}"
    rc=$?
    if [[ "${rc}" -eq 0 && -s "${SG_RAW}" ]] && jq -e '.results' "${SG_RAW}" >/dev/null 2>&1; then
      SG_RUNNER="docker ${SEMGREP_IMAGE} (${config_desc})"
    else
      attempts="docker rc=${rc}"
      rm -f "${SG_RAW}"
    fi
  fi

  if [[ "${SG_RUNNER}" == "none" && "${have_cli}" == "1" ]]; then
    local cli_version
    cli_version="$(semgrep --version --disable-version-check 2>/dev/null | tail -n 1)"
    echo "--- semgrep via local CLI ($(command -v semgrep) ${cli_version:-unknown version}; ${config_desc})" >>"${SG_ERR_LOG}"
    HOME="${QG_SEMGREP_HOME:-/tmp/staticanalysis-semgrep-home}" semgrep scan \
      "${configs[@]}" \
      --json \
      --quiet \
      --metrics=off \
      --disable-version-check \
      --output "${SG_RAW}" \
      "${SEMGREP_TARGETS[@]}" >/dev/null 2>>"${SG_ERR_LOG}"
    rc=$?
    if [[ "${rc}" -eq 0 && -s "${SG_RAW}" ]] && jq -e '.results' "${SG_RAW}" >/dev/null 2>&1; then
      SG_RUNNER="local semgrep CLI ${cli_version:-unknown version} (${config_desc})"
      if [[ -n "${cli_version}" && "${cli_version}" != "${SEMGREP_VERSION}" ]]; then
        echo "  warning: semgrep CLI ${cli_version} differs from the pinned ${SEMGREP_VERSION} (task tools:install)" >&2
      fi
    else
      attempts="${attempts:+${attempts}, }cli rc=${rc}"
      rm -f "${SG_RAW}"
    fi
  fi
  set -e
  show_stderr "${SG_ERR_LOG}"

  if [[ "${SG_RUNNER}" == "none" ]]; then
    tool_failed 3 "semgrep" "no runner produced a report (${attempts})" "${SG_ERR_LOG}"
    return 0
  fi

  local findings_tmp
  findings_tmp="$(mktemp)"
  jq '
    [ .results[] | {
        severity: .extra.severity,
        path: .path,
        line: .start.line,
        column: .start.col,
        check_id: .check_id,
        message: (.extra.message // "")
      }
    ]
  ' "${SG_RAW}" > "${findings_tmp}"
  wrap_report "semgrep" "${findings_tmp}" "${SG_RUNNER}" > "${SG_REPORT}"
  rm -f "${findings_tmp}"

  write_plan "semgrep" "${SG_REPORT}" "${SG_PLAN}"
  SG_ERRORS=$(jq '.summary.errors' "${SG_REPORT}")
  SG_TOTAL=$(jq '.summary.total'  "${SG_REPORT}")
  local scan_errors
  scan_errors=$(jq '(.errors // []) | length' "${SG_RAW}")

  local mark="$OK_MARK"; [[ "${SG_ERRORS}" -gt 0 ]] && mark="$FAIL_MARK"
  step_done 3 "semgrep" "$mark" "$(detail_str "$SG_TOTAL" "$SG_ERRORS" "$rc") via ${SG_RUNNER}${attempts:+ after failed ${attempts}}"
  if [[ "${scan_errors}" -gt 0 ]]; then
    echo "  semgrep reported ${scan_errors} scan error(s) (e.g. unparsable files); see ${SG_ERR_LOG}" >&2
  fi
}

# ---------- run all -----------------------------------------------------------

run_vet
run_staticcheck
run_semgrep

TOTAL_ERRORS=$((VET_ERRORS + SC_ERRORS + SG_ERRORS))
TOTAL_FINDINGS=$((VET_TOTAL + SC_TOTAL + SG_TOTAL))

echo
if [[ ${#TOOL_FAILURES[@]} -gt 0 ]]; then
  printf "staticanalysis: %d tool failure(s); the gate did not run completely:\n" "${#TOOL_FAILURES[@]}" >&2
  printf "  - %s\n" "${TOOL_FAILURES[@]}" >&2
  printf "staticanalysis: %d findings, %d errors from the tools that ran. FAILED.\n" \
    "${TOTAL_FINDINGS}" "${TOTAL_ERRORS}"
  exit 2
fi
if [[ "${TOTAL_ERRORS}" -gt 0 ]]; then
  printf "staticanalysis: %d findings, %d errors. Commit blocked.\n" \
    "${TOTAL_FINDINGS}" "${TOTAL_ERRORS}"
  exit 1
fi
printf "staticanalysis: %d findings, 0 errors. OK.\n" "${TOTAL_FINDINGS}"
exit 0
