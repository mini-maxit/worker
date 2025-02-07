# Worker Service

## Overview

The Worker Service is a message-driven application that listens to a RabbitMQ queue named `worker_queue`. Its primary function is to process messages that contain details about tasks to execute. Upon receiving a message, the worker gathers required files from a file storage service and processes them accordingly. The results are stored using the file storage service and sent back to the backend service.

## Message Structure

### Message Headers

- `x-retry-count=0`: This header tracks the number of times a message has been retried. If a message fails to process after 3 retries, it will be dropped.

### Message Properties

- `reply_to=test`: This property specifies the queue to which the worker will send the response.

### Message Body

The body of the message is a JSON object with the following structure:

```json
{
  "message_id": "adsa",
  "task_id": 123,
  "user_id": 1,
  "submission_number": 1,
  "language_type": "CPP",
  "language_version": "20",
  "time_limits": [25],
  "memory_limits": [512]
}
```

## Processing Flow

1. **Message Reception**: The worker listens to the `worker_queue` for incoming messages.
2. **File Retrieval**: After parsing the message, the worker retrieves necessary files from the file storage service based on the provided `task_id`, `user_id`, and `user_solution_id`.
3. **Execution**: The worker delegates the task to the Solution Runner service for processing.
4. **Response Sending**: The worker sends a response back to the specified `backend_response_queue`, indicating the success or failure of the task execution.

## Response Structure

### Successful Response

Upon successful execution of the task, the worker sends a message to the specified backend queue. The response will have the following structure:

```json
{
  "message_id": "adsa",
  "result": {
    "Success": true,
    "StatusCode": 1,
    "Code": "Success",
    "Message": "solution executed successfully",
    "TestResults": [
      {
        "Passed": false,
        "ErrorMessage": "Difference at line 1:\nOutput:   Hello, World!\nExpected: Hello World!\n\n",
        "Order": 1
      },
      {
        "Passed": true,
        "ErrorMessage": "",
        "Order": 2
      }
    ]
  }
}
```

### Error Response

In case of an error, the worker will return an error message structured as follows:

```json
{
  "message_id": "adsa",
  "result": {
    "Success": false,
    "StatusCode": 3,
    "Code": "500",
    "Message": "Failed to process the message after 3 retries: Failed to retrieve solution package: solution file does not exist for user 1, submission 1 of task 123",
    "TestResults": null
  }
}
```

## Error Handling

The worker implements retry logic for processing messages. If a message fails to process after three attempts, it is dropped, and an error message is sent to the designated backend queue. Error messages will include relevant details to assist in troubleshooting.

## Development

We use pre-commit hooks together with popular linters to enchance our code. Some of them are not installed by default:

- `go-imports` - golang.org/x/tools/cmd/goimports
- `golangci-lint` - https://github.com/golangci/golangci-lint
