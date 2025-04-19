# Worker Service

## Overview

The Worker Service is a message-driven application that listens to a RabbitMQ queue named `worker_queue`. Its primary function is to process messages that contain details about tasks to execute. Upon receiving a message, the worker gathers required files from a file storage service and processes them accordingly. The results are stored using the file storage service and sent back to the backend service.

The worker service can process two types of messages:
1. **Task Message**: Contains details about the task to execute.
2. **Handshake Message**: Used to syncronize the worker with the backend service. Returns supported languages and versions.
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
  "task_id": 3,
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
  "has_result": true, // Indicates if the message was processed successfully and produced a result
  "payload": {
    // Payload structure will vary based on the type of message and has_result value
  }
}
```

### 1. Task Message

Basic Task Message structure
```json
{
  "type": "task",
  "message_id": "adsa",
  "has_result": true, // Indicates if the task was processed successfully
  "payload": {
    "StatusCode": 1, // Indicates the status of the task
    "Message": "solution executed successfully", // Message indicating the result of the task
    "TestResults": [ // Array of test results
      {
        "Passed": true, // Indicates if the test passed
        "StatusCode": 1, // Indicates the status of the test
        "ExecutionTime": 0.000000, // Execution time of the test
        "ErrorMessage": "", // Error message for any runetime errors that occurred during execution of this test case
        "Order": 1 // Order of the test case
      }
    ]
  }
}
```

**Invalid Language Type**
This message occurs when the worer receives a language type that is not supported. The worker will return an error message indicating that the language type is invalid.

```json
{
  "type": "task",
  "message_id": "adsa",
  "has_result": true,
  "payload": {
    "StatusCode": 4, // InitializationError
    "Message": "invalid language type",
    "TestResults": []
  }
}
```

**Invalid Language Version**
This message occurs when the worer receives a language version that is not supported. The worker will return an error message indicating that the language version is invalid.

```json
{
  "type": "task",
  "message_id": "adsa",
  "has_result": true,
  "payload": {
    "StatusCode": 4, // InitializationError
    "Message": "invalid version supplied",
    "TestResults": []
  }
}
```

**Internal Error**
This message occurs when the worker encounters an internal error while processing the task. The worker will return an error message indicating that an internal error occurred. For example when creating user output directory fails.
```json
{
  "type": "task",
  "message_id": "adsa",
  "has_result": true,
  "payload": {
    "StatusCode": 5, // InternalError
    "Message": "mkdir /some/path/to/directory: no such file or directory", // For example
    "TestResults": [] // Might not be empty depending on when the error occurred
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
  "has_result": true,
  "payload": {
    "StatusCode": 2, // TestFailed
    "Message": "1. solution execution failed",
    "TestResults": [
      {
        "Passed": false,
        "StatusCode": 5, // RuntimeError
        "ExecutionTime": 0.000000,
        "ErrorMessage": "Segmentation fault", // For example
        "Order": 1
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
  "has_result": true,
  "payload": {
    "StatusCode": 5, // CompilationError
    "Message": "exec: \"g++\": executable file found in $PATH", // For example",
    "TestResults": []
  }
}
```

**Success**
This message occurs when the worker successfully executes the task. The worker will return a success message indicating that the task was executed and all test cases passed.
```json
{
  "type": "task",
  "message_id": "adsa",
  "has_result": true,
  "payload": {
    "StatusCode": 1, // Success
    "Message": "solution executed successfully",
    "TestResults": [
      {
        "Passed": true,
        "StatusCode": 1, // TestCasePassed
        "ExecutionTime": 0.000000,
        "ErrorMessage": "",
        "Order": 1
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
  "has_result": true,
  "payload": {
    "StatusCode": 2,
    "Message": "1. solution executed successfully, 2. time limit exceeded, 3. memory limit exceeded, 4. runtime error, 5. output difference",
    "TestResults": [
      {
        "Passed": true,
        "StatusCode": 1,
        "ExecutionTime": 0.000000,
        "ErrorMessage": "",
        "Order": 1
      },
      {
        "Passed": false,
        "StatusCode": 3,
        "ExecutionTime": 0.000000,
        "ErrorMessage": "Solution timed out after x s",
        "Order": 2
      },
      {
        "Passed": false,
        "StatusCode": 4,
        "ExecutionTime": 0.000000,
        "ErrorMessage": "Solution exceeded memory limit of x MB",
        "Order": 3
      },
      {
        "Passed": false,
        "StatusCode": 5,
        "ExecutionTime": 0.000000,
        "ErrorMessage": "Segmentation fault",
        "Order": 4
      },
      {
        "Passed": false,
        "StatusCode": 2,
        "ExecutionTime": 0.000000,
        "ErrorMessage": "",
        "Order": 5
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
  "has_result": true,
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
  "has_result": true,
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
  "has_result": false,
  "payload": {
    "error": "error message"
  }
}
```

## Development

We use pre-commit hooks together with popular linters to enchance our code. Some of them are not installed by default:

- `go-imports` - golang.org/x/tools/cmd/goimports
- `golangci-lint` - https://github.com/golangci/golangci-lint
