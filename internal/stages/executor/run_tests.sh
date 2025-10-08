#!/usr/bin/env bash

set -o pipefail
shopt -s nullglob

# sanity checks
if [[ -z "$INPUT_DIR" ]]; then
  echo "ERROR: INPUT_DIR must be set" >&2
  exit 1
fi

if [[ -z "$OUTPUT_DIR" ]]; then
  echo "ERROR: OUTPUT_DIR must be set" >&2
  exit 1
fi

if [[ -z "$EXEC_RESULT_DIR" ]]; then
  echo "ERROR: EXEC_RESULT_DIR must be set" >&2
  exit 1
fi

if [[ -z "$ERROR_DIR" ]]; then
  echo "ERROR: ERROR_DIR must be set" >&2
  exit 1
fi

read -r -a times <<< "${TIME_LIMITS:-}"
read -r -a mems  <<< "${MEM_LIMITS:-}"

if (( ${#times[@]} != ${#mems[@]} )); then
  echo "ERROR: TIME_LIMITS and MEM_LIMITS must have same count" >&2
  exit 1
fi

mapfile -t inputs < <(ls "${INPUT_DIR}"/*.in 2>/dev/null | sort)
if (( ${#inputs[@]} == 0 )); then
  echo "ERROR: no .in files found in $INPUT_DIR" >&2
  exit 1
fi

for idx in "${!inputs[@]}"; do
  testno=$((idx+1))
  infile="${inputs[idx]}"
  tsec=${times[idx]}
  mlimit_kb=${mems[idx]}

  out="${OUTPUT_DIR}/${testno}.out"
  err="${ERROR_DIR}/${testno}.err"
  exec_result="${EXEC_RESULT_DIR}/${testno}.result"

  # run in subshell to isolate failure
  (
    start_ns=$(date +%s%N)

    timeout --preserve-status "${tsec}s" bash -c \
      "ulimit -v ${mlimit_kb} && \"$1\" < \"${infile}\"" \
      > "${out}" 2> "${err}"
    code=$?
    if   (( code == 143 )); then
      echo "Time limit exceeded after ${tsec}s"    >> "${err}"
    elif (( code == 134 )); then
      echo "Memory limit exceeded max ${mlimit_kb}KB"  >> "${err}"
    fi

    end_ns=$(date +%s%N)
    duration_ns=$((end_ns - start_ns))
    duration_sec=$(awk "BEGIN {printf \"%.6f\", ${duration_ns}/1000000000}")

    # Write exit code and duration to exec result file
    echo "$code $duration_sec" >> "${exec_result}"
  )

done
