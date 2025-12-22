#!/usr/bin/env bash

set -o pipefail
shopt -s nullglob

# sanity checks for required env vars
if [[ -z "${RUN_CMD:-}" ]]; then
  echo "ERROR: RUN_CMD must be set" >&2
  exit 1
fi

if [[ -z "${INPUT_FILES:-}" ]]; then
  echo "ERROR: INPUT_FILES must be set" >&2
  exit 1
fi

if [[ -z "${USER_OUTPUT_FILES:-}" ]]; then
  echo "ERROR: USER_OUTPUT_FILES must be set" >&2
  exit 1
fi

if [[ -z "${USER_EXEC_RESULT_FILES:-}" ]]; then
  echo "ERROR: USER_EXEC_RESULT_FILES must be set" >&2
  exit 1
fi

if [[ -z "${USER_ERROR_FILES:-}" ]]; then
  echo "ERROR: USER_ERROR_FILES must be set" >&2
  exit 1
fi

read -r -a times <<< "$TIME_LIMITS_MS"
read -r -a mems <<< "$MEM_LIMITS_KB"
read -r -a inputs <<< "$INPUT_FILES"
read -r -a user_outputs <<< "$USER_OUTPUT_FILES"
read -r -a user_errors <<< "$USER_ERROR_FILES"
read -r -a exec_results <<< "$USER_EXEC_RESULT_FILES"

# Basic count validations
if (( ${#times[@]} != ${#mems[@]} )); then
  echo "ERROR: TIME_LIMITS and MEM_LIMITS must have same count" >&2
  exit 1
fi

n=${#inputs[@]}
if (( ${#times[@]} != n )); then
  echo "ERROR: TIME/MEM count must match number of INPUT_FILES (${n})" >&2
  exit 1
fi

if (( ${#user_outputs[@]} != n )); then
  echo "ERROR: USER_OUTPUT_FILES count must match number of INPUT_FILES" >&2
  exit 1
fi

if (( ${#user_errors[@]} != n )); then
  echo "ERROR: USER_ERROR_FILES count must match number of INPUT_FILES" >&2
  exit 1
fi

if (( ${#exec_results[@]} != n )); then
  echo "ERROR: USER_EXEC_RESULT_FILES count must match number of INPUT_FILES" >&2
  exit 1
fi

# Ensure all input files exist before running
missing=0
for f in "${inputs[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "ERROR: input file not found: $f" >&2
    missing=1
  fi
done
if (( missing )); then
  exit 1
fi

for idx in "${!inputs[@]}"; do
  testno=$((idx+1))
  infile="${inputs[idx]}"
  tlimit_ms=${times[idx]}
  mlimit_kb=${mems[idx]}

  out="${user_outputs[idx]}"
  err="${user_errors[idx]}"
  exec_result="${exec_results[idx]}"

  # create parent directories if necessary
  mkdir -p "$(dirname "${out}")"
  mkdir -p "$(dirname "${err}")"
  mkdir -p "$(dirname "${exec_result}")"

  # run in subshell to isolate failure
  (
    # temp file to store peak memory from /usr/bin/time
    peak_mem_file=$(mktemp)

    tlimit_s=$(awk "BEGIN {print ${tlimit_ms}/1000}")

    # Run the command with timeout and track peak memory usage
    hard_kb=$(( mlimit_kb + mlimit_kb / 10 ))

    start_ns=$(date +%s%N)
    timeout --preserve-status "${tlimit_s}s" \
      /usr/bin/time -q -f "%M" -o "$peak_mem_file" \
      bash -lc "ulimit -Sv ${mlimit_kb}; ulimit -Hv ${hard_kb}; exec ${RUN_CMD}" \
      < "$infile" > "$out" 2> "$err"
    code=$?
    end_ns=$(date +%s%N)

    peak_kb=0
    if [[ -s "$peak_mem_file" ]]; then
      peak_kb=$(<"$peak_mem_file")
    fi
    rm -f "$peak_mem_file"

    duration_ns=$((end_ns - start_ns))
    duration_sec=$(awk "BEGIN {printf \"%.6f\", ${duration_ns}/1000000000}")

    # Write exit code, duration, and peak memory (KB) to exec result file
    echo "$code $duration_sec $peak_kb" > "${exec_result}"
  )

done
