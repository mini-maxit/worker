# Worker Service

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
"payload":
{
  "task_id": 1,
  "user_id": 1,
  "submission_number": 1,
  "language_type": "CPP",
  "language_version": "20",
  "time_limits": [25,25,25,25],
  "memory_limits": [512,512,512,512]
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
2. **File Retrieval**: After parsing the message, the worker retrieves necessary files from the file storage service based on the provided `task_id`, `user_id`, and `user_solution_id`.
3. **Execution**: The worker delegates the task to the Solution Runner service for processing.
4. **Response Sending**: The worker sends a response back to the specified `backend_response_queue`, indicating the success or failure of the task execution.

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
    "message": "1. solution execution failed",
    "test_results": [
      {
        "passed": false,
        "status_code": 5,
        "execution_time": 0.000000,
        "error_message": "Segmentation fault",
        "order": 1
      }
    ]
  }
}
```

Error messages for test case failures are as follows:
- **Output Different** error message will be empty.
- **Time Limit Exceeded** error message: "Solution timed out after x s"
- **Memory Limit Exceeded** error message: "Solution exceeded memory limit of x MB"
- **Runtime Error** error message: "Segmentation fault" for example

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
    "message": "1. solution executed successfully, 2. time limit exceeded, 3. memory limit exceeded, 4. runtime error, 5. output difference",
    "test_results": [
      {
        "passed": true,
        "status_code": 1,
        "execution_time": 0.000000,
        "error_message": "",
        "order": 1
      },
      {
        "passed": false,
        "status_code": 3,
        "execution_time": 0.000000,
        "error_message": "Solution timed out after x s",
        "order": 2
      },
      {
        "passed": false,
        "status_code": 4,
        "execution_time": 0.000000,
        "error_message": "Solution exceeded memory limit of x MB",
        "order": 3
      },
      {
        "passed": false,
        "status_code": 5,
        "execution_time": 0.000000,
        "error_message": "Segmentation fault",
        "order": 4
      },
      {
        "passed": false,
        "status_code": 2,
        "execution_time": 0.000000,
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
- **3 (TimeLimitExceeded)**: The solution exceeded the allowed time limit for the test case.
- **4 (MemoryLimitExceeded)**: The solution exceeded the allowed memory limit for the test case.
- **5 (RuntimeError)**: A runtime error occurred during the execution of the test case (e.g., segmentation fault).

## Development

We use pre-commit hooks together with popular linters to enhance our code. Some of them are not installed by default:

- `go-imports` - golang.org/x/tools/cmd/goimports
- `golangci-lint` - https://github.com/golangci/golangci-lint
