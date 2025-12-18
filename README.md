# Worker Service
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/mini-maxit/worker)
[![codecov](https://codecov.io/gh/mini-maxit/worker/graph/badge.svg?token=AS7XEYOXBB)](https://codecov.io/gh/mini-maxit/worker)
## Overview

The Worker Service is a message-driven application that listens to a RabbitMQ queue named `worker_queue`. Its primary function is to process messages that contain details about tasks to execute. Upon receiving a message, the worker gathers required files from a file storage service and processes them accordingly. The results are stored using the file storage service and sent back to the backend service.

The worker service can process three types of messages:
1. **Task Message**: Contains details about the task to execute.
2. **Handshake Message**: Used to synchronize the worker with the backend service. Returns supported languages and versions.
3. **Status Message**: Used to check the status of the worker.

## Message Structure

### Message Properties

- `reply_to=test`: This property specifies the queue to which the worker will send the response.

### Message Body

The body of the message is a JSON object with the following structure:

#### Task Message
```json
{
  "type": "task",
  "message_id": "adsa",
  "payload": {
    "language_type": "cpp",
    "language_version": "17",
    "submission_file": {
      "server_type": "file_storage",
      "bucket": "task1",
      "path": "Task/prog.cpp"
    },
    "test_cases": [
      {
        "order": 1,
        "input_file": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/inputs/1.txt"
        },
        "expected_output": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/outputs/1.txt"
        },
        "stdout_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/stdout/1.txt"
        },
        "stderr_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/stderr/1.err"
        },
        "diff_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/diff/1.diff"
        },
        "time_limit_ms": 2000,
        "memory_limit_kb": 65536
      },
      {
        "order": 2,
        "input_file": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/inputs/2.txt"
        },
        "expected_output": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/outputs/2.txt"
        },
        "stdout_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/stdout/2.txt"
        },
        "stderr_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/stderr/2.err"
        },
        "diff_result": {
          "server_type": "file_storage",
          "bucket": "task1",
          "path": "Task/diff/2.diff"
        },
        "time_limit_ms": 2000,
        "memory_limit_kb": 65536
      }
    ]
  }
}
```
#### Handshake Message
```json
{
"type": "handshake",
"message_id": "adsa",
"payload": {}
}
```

### Status Message
```json
{
"type": "status",
"message_id": "adsa",
"payload": {}
}
```

## Processing Flow

1. **Message Reception**: The worker listens to the `worker_queue` for incoming messages.
2. **File Retrieval**: After parsing the message, the worker retrieves the submission file and test case files (inputs and expected outputs) from the file storage service based on the file locations specified in the message.
3. **Execution**: The worker compiles (if needed) and executes the solution against each test case, enforcing time and memory limits.
4. **Verification**: The worker compares the output with expected results and evaluates test case outcomes.
5. **File Upload**: The worker uploads result files (stdout, stderr, diff) to the file storage service at the specified locations.
6. **Response Sending**: The worker sends a response back to the specified reply queue, indicating the success or failure of the task execution along with detailed test results.

## Response Structure
Below are the possible responses from the worker service to different types of messages.

Basic structure of the response message:
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {}
}
```
Based on the `ok` field, the response can be either a success or an error message.

### 1. Task Message

Basic Task Message structure
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 1,
    "message": "solution executed successfully",
    "test_results": [
      {
        "passed": true,
        "status_code": 1,
        "execution_time": 0.000000,
        "peak_memory": 2048,
        "exit_code": 0,
        "error_message": "",
        "order": 1
      }
    ]
  }
}
```

**Invalid Language Type**
This message occurs when the worker receives a language type that is not supported. The worker will return an error message indicating that the language type is invalid.

```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 4,
    "message": "invalid language type",
    "test_results": []
  }
}
```

**Invalid Language Version**
This message occurs when the worker receives a language version that is not supported. The worker will return an error message indicating that the language version is invalid.

```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 4,
    "message": "invalid version supplied",
    "test_results": []
  }
}
```

**Internal Error**
This message occurs when the worker encounters an internal error while processing the task. The worker will return an error message indicating that an internal error occurred. For example when creating user output directory fails.
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 5,
    "message": "mkdir /some/path/to/directory: no such file or directory",
    "test_results": []
  }
}
```

**Test Failed**
This message occurs when any of the test cases fail. The worker will return an error message summarizing the test case failures. The worker will also return the test results for each test case. Possible failing test case statuses are:
- `OutputDifferent`, StatusCode 2
- `TimeLimitExceeded`, StatusCode 3
- `MemoryLimitExceeded`, StatusCode 4
- `RuntimeError`, StatusCode 5

```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 2,
    "message": "1. solution returned non-zero exit code.",
    "test_results": [
      {
        "passed": false,
        "status_code": 5,
        "execution_time": 0.000000,
        "peak_memory": 1024,
        "exit_code": 11,
        "error_message": "Solution exited with non-zero exit code 11",
        "order": 1
      }
    ]
  }
}
```

Error messages for test case failures are as follows:
- **Output Different** error message will be empty.
- **Time Limit Exceeded** error message: "Solution timed out after x ms" (exit code 143)
- **Memory Limit Exceeded** error message: "Solution exceeded memory limit of x kb" (exit code 134)
- **Runtime Error** error message: "Solution exited with non-zero exit code x" for standard errors, or "Solution exit code 127, possibly exceeded memory limit of x kb" when the exit code is 127 (command not found, which may indicate memory limit preventing shared library loading)

**Compilation Error**
This message occurs when the worker encounters a compilation error while executing the task. The worker will return an error message indicating that a compilation error occurred.
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 5,
    "message": "exec: \"g++\": executable file found in $PATH",
    "test_results": []
  }
}
```

**Success**
This message occurs when the worker successfully executes the task. The worker will return a success message indicating that the task was executed and all test cases passed.
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 1,
    "message": "solution executed successfully",
    "test_results": [
      {
        "passed": true,
        "status_code": 1,
        "execution_time": 0.000000,
        "peak_memory": 2048,
        "exit_code": 0,
        "error_message": "",
        "order": 1
      }
    ]
  }
}
```

**Example message**
```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "status_code": 2,
    "message": "1. solution executed successfully, 2. time limit exceeded, 3. memory limit exceeded, 4. solution returned non-zero exit code, 5. output difference",
    "test_results": [
      {
        "passed": true,
        "status_code": 1,
        "execution_time": 0.001234,
        "peak_memory": 2048,
        "exit_code": 0,
        "error_message": "",
        "order": 1
      },
      {
        "passed": false,
        "status_code": 3,
        "execution_time": 5.000000,
        "peak_memory": 3072,
        "exit_code": 143,
        "error_message": "Solution timed out after 5000 ms",
        "order": 2
      },
      {
        "passed": false,
        "status_code": 4,
        "execution_time": 0.100000,
        "peak_memory": 524288,
        "exit_code": 134,
        "error_message": "Solution exceeded memory limit of 512000 kb",
        "order": 3
      },
      {
        "passed": false,
        "status_code": 5,
        "execution_time": 0.050000,
        "peak_memory": 1024,
        "exit_code": 11,
        "error_message": "Solution exited with non-zero exit code 11",
        "order": 4
      },
      {
        "passed": false,
        "status_code": 2,
        "execution_time": 0.020000,
        "peak_memory": 1536,
        "exit_code": 0,
        "error_message": "",
        "order": 5
      }
    ]
  }
}
```

#### Handshake Message
This message occurs when the worker receives a handshake message. The worker will return a message indicating the supported languages and versions.

```json
{
  "type": "handshake",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "languages": [
      {
        "name": "CPP",
        "versions": ["20", "17"]
      },
      {
        "name": "Python",
        "versions": ["3", "2"]
      }
    ]
  }
}
```

#### Status Message
This message occurs when the worker receives a status message. The worker will return a message indicating the status of the worker.

```json
{
  "type": "status",
  "message_id": "adsa",
  "ok": true,
  "payload": {
    "busy_workers": 1,
    "total_workers": 2,
    "worker_status": {
      "0": "idle",
      "1": "busy Processing message 1",
    }
  }
}
```

### Error Response

In case of an error, the worker will return an error message structured as follows:

```json
{
  "type": "task",
  "message_id": "adsa",
  "ok": false,
  "payload": {
    "error": "error message"
  }
}
```

## Status Codes

### Solution Status Codes
The `status_code` field in the response indicates the overall status of the solution execution. Below are the possible values:

- **1 (Success)**: The solution executed successfully without any errors and all test cases passed.
- **2 (TestFailed)**: Some test cases failed (e.g., output difference, timeout, memory limit exceeded, etc.).
- **3 (CompilationError)**: The solution failed to compile.
- **4 (InitializationError)**: The solution failed to initialize due to an invalid language, version, or other configuration issues.
- **5 (InternalError)**: An internal error occurred during the execution (e.g., failure to create a directory).

### Test Result Status Codes
The `status_code` field in the `test_results` array indicates the status of individual test cases. Below are the possible values:

- **1 (TestCasePassed)**: The test case passed successfully.
- **2 (OutputDifference)**: The output of the solution differs from the expected output.
- **3 (TimeLimitExceeded)**: The solution exceeded the allowed time limit for the test case (exit code 143).
- **4 (MemoryLimitExceeded)**: The solution exceeded the allowed memory limit for the test case (exit code 134).
- **5 (NonZeroExitCode)**: The solution exited with a non-zero exit code that is different than memory limit exceeded or time limit exceeded. This includes runtime errors (e.g., segmentation fault with exit code 11) and other error conditions (e.g., exit code 127 which may indicate memory limit preventing shared library loading).

## Development

We use pre-commit hooks together with popular linters to enhance our code. Some of them are not installed by default:

- `go-imports` - golang.org/x/tools/cmd/goimports
- `golangci-lint` - https://github.com/golangci/golangci-lint
