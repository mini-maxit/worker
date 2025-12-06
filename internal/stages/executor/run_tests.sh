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

read -r -a times <<< "${TIME_LIMITS:-}"
read -r -a mems  <<< "${MEM_LIMITS:-}"
read -r -a inputs <<< "${INPUT_FILES:-}"
read -r -a user_outputs <<< "${USER_OUTPUT_FILES:-}"
read -r -a user_errors <<< "${USER_ERROR_FILES:-}"
read -r -a exec_results <<< "${USER_EXEC_RESULT_FILES:-}"

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
  tsec=${times[idx]}
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
    start_ns=$(date +%s%N)

    timeout --preserve-status "${tsec}s" bash -c \
      "ulimit -v ${mlimit_kb} && $RUN_CMD" < "${infile}" \
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
