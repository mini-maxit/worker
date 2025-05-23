#!/usr/bin/env bash

set -o pipefail
shopt -s nullglob

# sanity checks
if [[ -z "$INPUT_DIR" ]]; then
  echo "ERROR: INPUT_DIR must be set" >&2
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
  err="${OUTPUT_DIR}/${testno}.err"

  # Measure start time in nanoseconds
  start_ns=$(date +%s%N)

  # run in subshell to isolate failure
  (
    timeout --preserve-status "${tsec}s" bash -c \
      "ulimit -v ${mlimit_kb} && exec \"$1\" < \"${infile}\"" \
      > "${out}" 2> "${err}"
    code=$?
    if   (( code == 143 )); then
      echo "Time Limit Exceeded"    >> "${err}"
    elif (( code == 139 )); then
      echo "Memory Limit Exceeded"  >> "${err}"
    fi
  )

  code=$?
  # Measure end time and calculate duration in seconds with microsecond precision
  end_ns=$(date +%s%N)
  duration_ns=$((end_ns - start_ns))
  duration_sec=$(awk "BEGIN {printf \"%.6f\", ${duration_ns}/1000000000}")

  # Write exit code and duration to exec result file
  echo "$code $duration_sec" >> "${OUTPUT_DIR}/${EXEC_RESULT_FILE}"
done
