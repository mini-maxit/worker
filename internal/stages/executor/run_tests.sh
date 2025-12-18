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

    start_ns=$(date +%s%N)

    # Run the command with timeout and track peak memory usage
    hard_kb=$(( mlimit_kb + mlimit_kb / 10 ))
    timeout --preserve-status "${tlimit_s}s" \
      /usr/bin/time -f "%M" -o "${peak_mem_file}" \
      bash -c "ulimit -S -v ${mlimit_kb} && ulimit -H -v ${hard_kb} && ${RUN_CMD}" < "$infile" \
      > "${out}" 2> "${err}"

    end_ns=$(date +%s%N)
    code=$?

    if [[ ! -s "${peak_mem_file}" ]]; then
      peak_kb=0
    else
      peak_kb=$(grep -o '[0-9]*' "${peak_mem_file}" | tail -n 1)
    fi
    rm -f "${peak_mem_file}"

    sig=0
    if (( code > 128 )); then
      sig=$((code - 128))
    fi

    is_mle=0
    if (( sig == 15 )); then
      echo "Time limit exceeded after ${tlimit_ms}ms" >> "${err}"
    # SIGKILL(9), SIGSEGV(11), SIGABRT(6), SIGBUS(10) possibly mem limit exceeded, but this is NOT guaranteed by POSIX.
    elif (( sig == 9 || sig == 11 || sig == 6 || sig == 10 )); then
      # Require that we were close to the memory limit
      threshold_kb=$(( mlimit_kb * 9 / 10 ))
      if (( peak_kb >= threshold_kb )); then
      is_mle=1
      fi
    fi

    if ((is_mle)); then
      echo "Memory limit exceeded max ${mlimit_kb}KB" >> "${err}"
      code=134
    fi

    duration_ns=$((end_ns - start_ns))
    duration_sec=$(awk "BEGIN {printf \"%.6f\", ${duration_ns}/1000000000}")

    # Write exit code, duration, and peak memory (KB) to exec result file
    echo "$code $duration_sec $peak_kb" > "${exec_result}"
  )

done
