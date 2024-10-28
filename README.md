# Worker Service

## Overview

The Worker Service is a message-driven application that listens to a RabbitMQ queue named `worker_queue`. Its primary function is to process messages that contain details about tasks to execute. Upon receiving a message, the worker gathers required files from a file storage service and processes them accordingly. The results are then sent back to a specified backend queue.

## Message Structure

### Message Headers

- `x-retry-count=0`: This header tracks the number of times a message has been retried. If a message fails to process after 3 retries, it will be dropped.

### Message Body

The body of the message is a JSON object with the following structure:

```json
{
  "message_id": "adsa",
  "backend_response_queue": "test",
  "task_id": 123,
  "user_id": 1,
  "user_solution_id": 1,
  "language_type": "CPP",
  "language_version": "20",
  "solution_file_name": "solution.cpp",
  "time_limits": [25],
  "memory_limits": [512],
  "output_dir_name": "outputs",
  "input_dir_name": "inputs"
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
  "task_id": 123,
  "user_id": 1,
  "user_solution_id": 1,
  "result": {
    "Success": true,
    "StatusCode": 1,
    "Code": "Success",
    "Message": "solution executed successfully",
    "TestResults": [
      {
        "InputFile": "/tmp/temp302885701/Task/inputs/1.in",
        "ExpectedFile": "/tmp/temp302885701/Task/outputs/1.out",
        "ActualFile": "/tmp/temp302885701/Task/user-output/1.out",
        "Passed": false,
        "ErrorMessage": "Difference at line 1:\nOutput:   Hello, World!\nExpected: Hello World!\n\n",
        "Order": 1
      },
      {
        "InputFile": "/tmp/temp302885701/Task/inputs/2.in",
        "ExpectedFile": "/tmp/temp302885701/Task/outputs/2.out",
        "ActualFile": "/tmp/temp302885701/Task/user-output/2.out",
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
  "task_id": 123,
  "user_id": 1,
  "user_solution_id": 1,
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
