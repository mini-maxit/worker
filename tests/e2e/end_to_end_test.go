//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/rabbitmq"
	"github.com/mini-maxit/worker/internal/rabbitmq/channel"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	testRabbitMQURL   = "amqp://guest:guest@localhost:5672/"
	fileStorageURL    = "http://localhost:8888"
	testBucket        = "e2e-test"
	workerQueueName   = "worker_queue"
	responseQueueName = "response_queue"
	messageTimeout    = 120 * time.Second
)

// Helper functions for file storage operations
func uploadToFileStorage(t *testing.T, bucket, path, content string) {
	t.Helper()
	// Extract directory from path for the prefix parameter
	var prefix string
	if idx := bytes.LastIndexByte([]byte(path), '/'); idx != -1 {
		prefix = path[:idx]
	} else {
		prefix = ""
	}

	// Create temporary file to upload
	tmpFile, err := os.CreateTemp("", "e2e-upload-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Build upload URL: /buckets/{bucket}/upload-multiple?prefix=<prefix>
	uploadURL := fmt.Sprintf("%s/buckets/%s/upload-multiple", fileStorageURL, bucket)
	if prefix != "" {
		uploadURL = fmt.Sprintf("%s?prefix=%s", uploadURL, url.QueryEscape(prefix))
	}

	// Prepare multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open temp file: %v", err)
	}
	defer file.Close()

	// Get just the filename (last part of path)
	filename := path
	if idx := bytes.LastIndexByte([]byte(path), '/'); idx != -1 {
		filename = path[idx+1:]
	}

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		t.Fatalf("failed to copy file to form: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req, err := http.NewRequest("POST", uploadURL, body)
	if err != nil {
		t.Fatalf("failed to create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to upload file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("failed to upload file, status: %d, body: %s", resp.StatusCode, string(body))
	}
}

func cleanupBucket(t *testing.T, bucket string) {
	t.Helper()
	// File storage cleanup - delete bucket
	// This is best effort, so we don't fail if it doesn't work
	deleteURL := fmt.Sprintf("%s/buckets/%s", fileStorageURL, bucket)
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		t.Logf("warning: failed to create DELETE request for bucket: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("warning: failed to delete bucket: %v", err)
		return
	}
	defer resp.Body.Close()
}

func createBucket(t *testing.T, bucket string) {
	t.Helper()
	// Create bucket using POST to /buckets with JSON body
	createURL := fmt.Sprintf("%s/buckets", fileStorageURL)

	// Prepare JSON payload
	payload := map[string]string{"name": bucket}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal bucket creation payload: %v", err)
	}

	req, err := http.NewRequest("POST", createURL, bytes.NewBuffer(payloadJSON))
	if err != nil {
		t.Fatalf("failed to create bucket creation request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to create bucket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("warning: failed to create bucket, status: %d, body: %s", resp.StatusCode, string(body))
		// Don't fail - bucket might already exist
	}
}

// Test case structure
type e2eTestCase struct {
	name                string
	language            string
	languageVersion     string
	program             string
	programPath         string
	inputFiles          map[string]string // order -> content
	expectedOutputFiles map[string]string // order -> content
	timeLimitMs         int64
	memoryLimitKB       int64
	expectedStatus      solution.ResultStatus
	expectedTestResults map[int]solution.TestCaseStatus // order -> expected status
}

func setupTestCase(t *testing.T, tc e2eTestCase, bucket string) messages.TaskQueueMessage {
	t.Helper()

	// Create bucket first
	createBucket(t, bucket)

	// Upload program
	uploadToFileStorage(t, bucket, tc.programPath, tc.program)

	// Build test cases
	var testCases []messages.TestCase
	for orderStr, inputContent := range tc.inputFiles {
		order := mustAtoi(t, orderStr)
		inputPath := fmt.Sprintf("inputs/%s.txt", orderStr)
		outputPath := fmt.Sprintf("outputs/%s.txt", orderStr)
		stdoutPath := fmt.Sprintf("stdout/%s.txt", orderStr)
		stderrPath := fmt.Sprintf("stderr/%s.err", orderStr)
		diffPath := fmt.Sprintf("diff/%s.diff", orderStr)

		// Upload input file
		uploadToFileStorage(t, bucket, inputPath, inputContent)

		// Upload expected output file
		expectedOutput := tc.expectedOutputFiles[orderStr]
		uploadToFileStorage(t, bucket, outputPath, expectedOutput)

		testCase := messages.TestCase{
			Order: order,
			InputFile: messages.FileLocation{
				ServerType: "file_storage",
				Bucket:     bucket,
				Path:       inputPath,
			},
			ExpectedOutput: messages.FileLocation{
				ServerType: "file_storage",
				Bucket:     bucket,
				Path:       outputPath,
			},
			StdOutResult: messages.FileLocation{
				ServerType: "file_storage",
				Bucket:     bucket,
				Path:       stdoutPath,
			},
			StdErrResult: messages.FileLocation{
				ServerType: "file_storage",
				Bucket:     bucket,
				Path:       stderrPath,
			},
			DiffResult: messages.FileLocation{
				ServerType: "file_storage",
				Bucket:     bucket,
				Path:       diffPath,
			},
			TimeLimitMs:   tc.timeLimitMs,
			MemoryLimitKB: tc.memoryLimitKB,
		}
		testCases = append(testCases, testCase)
	}

	return messages.TaskQueueMessage{
		LanguageType:    tc.language,
		LanguageVersion: tc.languageVersion,
		SubmissionFile: messages.FileLocation{
			ServerType: "file_storage",
			Bucket:     bucket,
			Path:       tc.programPath,
		},
		TestCases: testCases,
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		t.Fatalf("failed to convert %s to int: %v", s, err)
	}
	return i
}

func waitForResponse(t *testing.T, msgs <-chan amqp.Delivery, timeout time.Duration) *messages.ResponseQueueMessage {
	t.Helper()
	select {
	case msg := <-msgs:
		var responseMsg messages.ResponseQueueMessage
		if err := json.Unmarshal(msg.Body, &responseMsg); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		return &responseMsg
	case <-time.After(timeout):
		t.Fatal("timeout waiting for response")
		return nil
	}
}

func sendTaskMessage(t *testing.T, channel channel.Channel, task messages.TaskQueueMessage, messageID, responseQueue string) {
	t.Helper()

	payload, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task: %v", err)
	}

	queueMsg := messages.QueueMessage{
		Type:      constants.QueueMessageTypeTask,
		MessageID: messageID,
		Payload:   json.RawMessage(payload),
	}

	messageJSON, err := json.Marshal(queueMsg)
	if err != nil {
		t.Fatalf("failed to marshal queue message: %v", err)
	}

	err = channel.Publish(
		"",
		workerQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        messageJSON,
			ReplyTo:     responseQueue,
		},
	)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
}

// C++ test cases

func TestE2E_CPP_Valid(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "CPP Valid Submission",
		language:        "cpp",
		languageVersion: "17",
		program: `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    cout << a + b << endl;
    return 0;
}`,
		programPath: "program.cpp",
		inputFiles: map[string]string{
			"1": "5 3\n",
			"2": "10 20\n",
			"3": "-5 5\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
			"2": "30\n",
			"3": "0\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  65536,
		expectedStatus: solution.Success,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.TestCasePassed,
			2: solution.TestCasePassed,
			3: solution.TestCasePassed,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_CPP_CompilationError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "CPP Compilation Error",
		language:        "cpp",
		languageVersion: "17",
		program: `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    cout << a + b << endl
    // Missing semicolon and return statement
}`,
		programPath: "program.cpp",
		inputFiles: map[string]string{
			"1": "5 3\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
		},
		timeLimitMs:         2000,
		memoryLimitKB:       65536,
		expectedStatus:      solution.CompilationError,
		expectedTestResults: map[int]solution.TestCaseStatus{},
	}

	runE2ETest(t, tc)
}

func TestE2E_CPP_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "CPP Timeout",
		language:        "cpp",
		languageVersion: "17",
		program: `#include <iostream>
#include <chrono>
#include <thread>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    // Sleep for 5 seconds to trigger timeout
    this_thread::sleep_for(chrono::seconds(5));
    cout << a + b << endl;
    return 0;
}`,
		programPath: "program.cpp",
		inputFiles: map[string]string{
			"1": "5 3\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
		},
		timeLimitMs:    1000, // 1 second limit
		memoryLimitKB:  65536,
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.TimeLimitExceeded,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_CPP_MemoryLimitExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "CPP Memory Limit Exceeded",
		language:        "cpp",
		languageVersion: "17",
		program: `#include <iostream>
#include <vector>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    // Allocate large amount of memory (100MB)
    vector<int> v(25 * 1024 * 1024); // 25M ints = 100MB
    cout << a + b << endl;
    return 0;
}`,
		programPath: "program.cpp",
		inputFiles: map[string]string{
			"1": "5 3\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  10240, // 10MB limit
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.MemoryLimitExceeded,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_CPP_OutputDifference(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "CPP Output Difference",
		language:        "cpp",
		languageVersion: "17",
		program: `#include <iostream>
using namespace std;

int main() {
    int a, b;
    cin >> a >> b;
    // Wrong output - multiplication instead of addition
    cout << a * b << endl;
    return 0;
}`,
		programPath: "program.cpp",
		inputFiles: map[string]string{
			"1": "5 3\n",
			"2": "10 20\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
			"2": "30\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  65536,
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.OutputDifference,
			2: solution.OutputDifference,
		},
	}

	runE2ETest(t, tc)
}

// Python test cases

func TestE2E_Python_Valid(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "Python Valid Submission",
		language:        "python",
		languageVersion: "3.11",
		program: `a, b = map(int, input().split())
print(a + b)`,
		programPath: "program.py",
		inputFiles: map[string]string{
			"1": "5 3\n",
			"2": "10 20\n",
			"3": "-5 5\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
			"2": "30\n",
			"3": "0\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  65536,
		expectedStatus: solution.Success,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.TestCasePassed,
			2: solution.TestCasePassed,
			3: solution.TestCasePassed,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_Python_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "Python Timeout",
		language:        "python",
		languageVersion: "3.11",
		program: `import time

a, b = map(int, input().split())
# Sleep for 5 seconds to trigger timeout
time.sleep(5)
print(a + b)`,
		programPath: "program.py",
		inputFiles: map[string]string{
			"1": "5 3\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
		},
		timeLimitMs:    1000, // 1 second limit
		memoryLimitKB:  65536,
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.TimeLimitExceeded,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_Python_MemoryLimitExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "Python Memory Limit Exceeded",
		language:        "python",
		languageVersion: "3.11",
		program: `a, b = map(int, input().split())
# Allocate large amount of memory (100MB)
large_list = [0] * (25 * 1024 * 1024)
print(a + b)`,
		programPath: "program.py",
		inputFiles: map[string]string{
			"1": "5 3\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  10240, // 10MB limit
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.MemoryLimitExceeded,
		},
	}

	runE2ETest(t, tc)
}

func TestE2E_Python_OutputDifference(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	tc := e2eTestCase{
		name:            "Python Output Difference",
		language:        "python",
		languageVersion: "3.11",
		program: `a, b = map(int, input().split())
# Wrong output - multiplication instead of addition
print(a * b)`,
		programPath: "program.py",
		inputFiles: map[string]string{
			"1": "5 3\n",
			"2": "10 20\n",
		},
		expectedOutputFiles: map[string]string{
			"1": "8\n",
			"2": "30\n",
		},
		timeLimitMs:    2000,
		memoryLimitKB:  65536,
		expectedStatus: solution.TestFailed,
		expectedTestResults: map[int]solution.TestCaseStatus{
			1: solution.OutputDifference,
			2: solution.OutputDifference,
		},
	}

	runE2ETest(t, tc)
}

// Common test runner

func runE2ETest(t *testing.T, tc e2eTestCase) {
	t.Log("Starting test:", tc.name)

	// Create unique bucket for this test
	bucket := fmt.Sprintf("%s-%d", testBucket, time.Now().UnixNano())
	defer cleanupBucket(t, bucket)

	// Setup RabbitMQ connection
	cfg := &config.Config{
		RabbitMQURL: testRabbitMQURL,
	}

	conn := rabbitmq.NewRabbitMqConnection(cfg)
	defer conn.Close()

	channel := rabbitmq.NewRabbitMQChannel(conn)

	// Declare response queue
	responseQueue, err := channel.QueueDeclare(
		fmt.Sprintf("%s-%d", responseQueueName, time.Now().UnixNano()),
		false, // durable
		true,  // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		t.Fatalf("failed to declare response queue: %v", err)
	}

	// Start consuming responses
	msgs, err := channel.Consume(
		responseQueue.Name,
		"",    // consumer
		true,  // autoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		t.Fatalf("failed to start consuming: %v", err)
	}

	// Setup test case files
	task := setupTestCase(t, tc, bucket)

	// Send task message
	messageID := fmt.Sprintf("test-msg-%d", time.Now().UnixNano())
	t.Logf("Sending task message with ID: %s", messageID)
	sendTaskMessage(t, channel, task, messageID, responseQueue.Name)

	// Wait for response
	t.Log("Waiting for response...")
	response := waitForResponse(t, msgs, messageTimeout)

	// Verify response
	t.Log("Verifying response...")
	if response.MessageID != messageID {
		t.Errorf("expected message ID %s, got %s", messageID, response.MessageID)
	}

	if response.Type != constants.QueueMessageTypeTask {
		t.Errorf("expected type %s, got %s", constants.QueueMessageTypeTask, response.Type)
	}

	// Parse result
	var result solution.Result
	if err := json.Unmarshal(response.Payload, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Verify status code
	if result.StatusCode != tc.expectedStatus {
		t.Errorf("expected status %d, got %d. Message: %s", tc.expectedStatus, result.StatusCode, result.Message)
	}

	// For compilation errors, we don't check test results
	if tc.expectedStatus == solution.CompilationError {
		if result.StatusCode == solution.CompilationError {
			t.Log("Compilation error detected as expected")
			return
		}
	}

	// Verify test results
	if len(result.TestResults) != len(tc.expectedTestResults) {
		t.Errorf("expected %d test results, got %d", len(tc.expectedTestResults), len(result.TestResults))
	}

	for _, testResult := range result.TestResults {
		expectedStatus, exists := tc.expectedTestResults[testResult.Order]
		if !exists {
			t.Errorf("unexpected test result for order %d", testResult.Order)
			continue
		}

		if testResult.StatusCode != expectedStatus {
			t.Errorf("test case %d: expected status %d, got %d. Error: %s",
				testResult.Order, expectedStatus, testResult.StatusCode, testResult.ErrorMessage)
		}

		// Check if test passed flag matches expected status
		expectedPassed := expectedStatus == solution.TestCasePassed
		if testResult.Passed != expectedPassed {
			t.Errorf("test case %d: expected passed=%v, got passed=%v",
				testResult.Order, expectedPassed, testResult.Passed)
		}
	}

	t.Logf("Test %s completed successfully", tc.name)
}
