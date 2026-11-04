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
#   QG_VERBOSE=1       — stream tool stderr live (default: log to .staticanalysis/<tool>.stderr.log)
#
# Outputs (all under .staticanalysis/):
#   .staticanalysis/report.<tool>.json   findings
#   .staticanalysis/plan.<tool>.md       grouped markdown plan
#   .staticanalysis/<tool>.stderr.log    suppressed tool stderr (for debugging)
#
# Override location with QG_OUT_DIR=path.
#
# Exit codes: 0 clean, 1 ERROR-severity finding(s) in any tool.

set -euo pipefail

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
  mapfile -t STAGED_FILES < <(git diff --cached --name-only --diff-filter=ACM 2>/dev/null | grep -E '\.go$' || true)
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
  local tool="$1" findings_file="$2"
  jq -n \
    --slurpfile findings "${findings_file}" \
    --arg tool "${tool}" \
    --arg mode "${MODE}" \
    --arg ts "${TS}" \
    '
    ($findings[0] // []) as $all
    | {
        tool: $tool, generated_at: $ts, mode: $mode,
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

# stderr_redirect TARGET_LOG → returns the shell redirection target.
# In verbose mode, leak stderr to terminal; otherwise pipe to log file.
stderr_target() {
  if [[ "${QG_VERBOSE:-0}" == "1" ]]; then
    echo "/dev/stderr"
  else
    echo "$1"
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
  step_start 1 "go-vet"

  local err_target
  err_target="$(stderr_target "${VET_ERR_LOG}")"
  set +e
  GOCACHE="${GOCACHE:-/tmp/go-build-cache}" go vet "${GO_PKGS[@]}" >"${VET_RAW}" 2>"${err_target}"
  local rc=$?
  set -e
  # vet emits diagnostics on stderr; treat captured log as the raw report too.
  [[ -s "${VET_ERR_LOG}" && ! -s "${VET_RAW}" ]] && cp "${VET_ERR_LOG}" "${VET_RAW}"

  local findings_tmp
  findings_tmp="$(mktemp)"
  awk '
    /^#/ { next }
    /^[[:space:]]*$/ { next }
    {
      n = index($0, ":")
      if (n == 0) next
      file = substr($0, 1, n-1)
      rest = substr($0, n+1)
      n2 = index(rest, ":")
      if (n2 == 0) next
      line = substr(rest, 1, n2-1)
      rest2 = substr(rest, n2+1)
      n3 = index(rest2, ":")
      col = "0"
      msg = rest2
      if (n3 > 0 && substr(rest2,1,n3-1) ~ /^[0-9]+$/) {
        col = substr(rest2, 1, n3-1)
        msg = substr(rest2, n3+1)
      }
      sub(/^[[:space:]]+/, "", msg)
      gsub(/"/, "\\\"", msg)
      printf "{\"severity\":\"ERROR\",\"path\":\"%s\",\"line\":%s,\"column\":%s,\"check_id\":\"go-vet\",\"message\":\"%s\"}\n", file, line, col, msg
    }
  ' "${VET_RAW}" | jq -s '.' > "${findings_tmp}"

  wrap_report "go-vet" "${findings_tmp}" > "${VET_REPORT}"
  rm -f "${findings_tmp}"
  write_plan "go-vet" "${VET_REPORT}" "${VET_PLAN}"

  VET_ERRORS=$(jq '.summary.errors' "${VET_REPORT}")
  VET_TOTAL=$(jq '.summary.total'  "${VET_REPORT}")
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
    step_done 2 "staticcheck" "$SKIP_MARK" "not installed"
    return 0
  fi
  step_start 2 "staticcheck"

  local err_target
  err_target="$(stderr_target "${SC_ERR_LOG}")"
  set +e
  XDG_CACHE_HOME="${XDG_CACHE_HOME:-/tmp/staticanalysis-cache}" staticcheck -f json "${GO_PKGS[@]}" >"${SC_RAW}" 2>"${err_target}"
  local rc=$?
  set -e

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
  step_start 3 "semgrep"

  local err_target
  err_target="$(stderr_target "${SG_ERR_LOG}")"
  [[ "${QG_VERBOSE:-0}" != "1" ]] && : >"${SG_ERR_LOG}"
  rm -f "${SG_REPORT}" "${SG_RAW}"

  set +e
  local rc=0
  if docker info >/dev/null 2>/dev/null; then
    docker run --rm -v "${PWD}:/src" -w /src semgrep/semgrep semgrep scan \
      --config=.semgrep.yml \
      --config=p/golang \
      --config=p/gosec \
      --config=p/secrets \
      --json \
      --quiet \
      --metrics=off \
      --output "${SG_RAW}" \
      "${SEMGREP_TARGETS[@]}" >/dev/null 2>>"${err_target}"
    rc=$?
  else
    rc=127
  fi

  if [[ ! -s "${SG_RAW}" && -x "$(command -v semgrep 2>/dev/null)" ]]; then
    HOME="${QG_SEMGREP_HOME:-/tmp/staticanalysis-semgrep-home}" semgrep scan \
      --config=.semgrep.yml \
      --json \
      --quiet \
      --metrics=off \
      --disable-version-check \
      --output "${SG_RAW}" \
      "${SEMGREP_TARGETS[@]}" >/dev/null 2>>"${err_target}"
    rc=$?
  fi
  set -e

  local findings_tmp normalized_tmp
  findings_tmp="$(mktemp)"
  normalized_tmp="$(mktemp)"

  if [[ ! -s "${SG_RAW}" ]]; then
    echo "[]" > "${findings_tmp}"
    wrap_report "semgrep" "${findings_tmp}" > "${SG_REPORT}"
  else
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
    wrap_report "semgrep" "${findings_tmp}" > "${normalized_tmp}"
    cat "${normalized_tmp}" > "${SG_REPORT}"
  fi

  write_plan "semgrep" "${SG_REPORT}" "${SG_PLAN}"
  SG_ERRORS=$(jq '.summary.errors' "${SG_REPORT}")
  SG_TOTAL=$(jq '.summary.total'  "${SG_REPORT}")
  rm -f "${findings_tmp}" "${normalized_tmp}"

  local mark="$OK_MARK"; [[ "${SG_ERRORS}" -gt 0 ]] && mark="$FAIL_MARK"
  step_done 3 "semgrep" "$mark" "$(detail_str "$SG_TOTAL" "$SG_ERRORS" "$rc")"
}

# ---------- run all -----------------------------------------------------------

run_vet
run_staticcheck
run_semgrep

TOTAL_ERRORS=$((VET_ERRORS + SC_ERRORS + SG_ERRORS))
TOTAL_FINDINGS=$((VET_TOTAL + SC_TOTAL + SG_TOTAL))

echo
if [[ "${TOTAL_ERRORS}" -gt 0 ]]; then
  printf "staticanalysis: %d findings, %d errors. Commit blocked.\n" \
    "${TOTAL_FINDINGS}" "${TOTAL_ERRORS}"
  exit 1
fi
printf "staticanalysis: %d findings, 0 errors. OK.\n" "${TOTAL_FINDINGS}"
exit 0
