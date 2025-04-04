package tests_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/services"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/rabbitmq"
	"github.com/mini-maxit/worker/tests"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

// Struct for validating response payload.
type TaskResponsePayload struct {
	OutputDir   string                `json:"output_dir"`
	StatusCode  int                   `json:"status_code"`
	Code        string                `json:"code"`
	Message     string                `json:"message"`
	TestResults []solution.TestResult `json:"test_results"`
}

type ExpectedTaskResponse struct {
	Type      string              `json:"type"`
	MessageID string              `json:"message_id"`
	Payload   TaskResponsePayload `json:"payload"`
}

type HandshakeResponsePayload struct {
	Languages []LanguageConfig `json:"languages"`
}

type ExpectedHandshakeResponse struct {
	Type      string                   `json:"type"`
	MessageID string                   `json:"message_id"`
	Payload   HandshakeResponsePayload `json:"payload"`
}

type LanguageConfig struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type StatusResponsePayload struct {
	BusyWorkers  int               `json:"busy_workers"`
	TotalWorkers int               `json:"total_workers"`
	WorkerStatus map[string]string `json:"worker_status"`
}

type ExpecredStatusResponse struct {
	Type      string                `json:"type"`
	MessageID string                `json:"message_id"`
	Payload   StatusResponsePayload `json:"payload"`
}

const responseQueueName = "reply_to"

func generateQueueMessage(test tests.TestType, language, version string) []byte {
	var payload map[string]interface{}
	var msgType string

	//nolint:exhaustive
	switch test {
	case tests.Handshake:
		payload = map[string]interface{}{}
		msgType = "handshake"
	case tests.Status:
		payload = map[string]interface{}{}
		msgType = "status"
	case tests.LongTaskMessage:
		payload = map[string]interface{}{
			"task_id":           1,
			"user_id":           1,
			"submission_number": tests.CPPFailedTimeLimitExceeded,
			"language_type":     "CPP",
			"language_version":  "20",
			"time_limits":       []int{20},
			"memory_limits":     []int{512},
			"chroot_dir_path":   fmt.Sprintf("%s/Task_1_1_%d", tests.MockTmpDir, tests.CPPFailedTimeLimitExceeded),
			"use_chroot":        "false",
		}
		msgType = "task"
	default:
		payload = map[string]interface{}{
			"task_id":           1,
			"user_id":           1,
			"submission_number": test,
			"language_type":     language,
			"language_version":  version,
			"time_limits":       []int{2},
			"memory_limits":     []int{512},
			"chroot_dir_path":   fmt.Sprintf("%s/Task_1_1_%d", tests.MockTmpDir, test),
			"use_chroot":        "false",
		}
		msgType = "task"
	}

	message := map[string]interface{}{
		"type":       msgType,
		"message_id": "adsa",
		"payload":    payload,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return nil
	}

	return messageBytes
}

func declareResponseQueue(ch *amqp.Channel, queueName string) (amqp.Queue, error) {
	return ch.QueueDeclare(queueName, false, false, false, false, nil)
}

func consumeResponse(ch *amqp.Channel, queueName string) (<-chan amqp.Delivery, error) {
	return ch.Consume(queueName, "", true, false, false, false, nil)
}

func publishMessage(ch *amqp.Channel, message []byte) error {
	return ch.Publish(
		"",             // exchange
		"worker_queue", // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        message,
			ReplyTo:     responseQueueName,
		})
}

func isSuccess(actual ExpectedTaskResponse) bool {
	return actual.Payload.StatusCode == int(solution.Success) &&
		strings.Contains(actual.Payload.Message, constants.SolutionMessageSuccess) &&
		len(actual.Payload.TestResults) == 1 && actual.Payload.TestResults[0].Passed
}

func isTimeLimitExceeded(actual ExpectedTaskResponse) bool {
	return actual.Payload.StatusCode == int(solution.TimeLimitExceeded) &&
		strings.Contains(actual.Payload.Message, constants.SolutionMessageTimeout) &&
		len(actual.Payload.TestResults) == 1 &&
		!actual.Payload.TestResults[0].Passed &&
		actual.Payload.TestResults[0].ErrorMessage == constants.TestMessageTimeLimitExceeded
}

func isCompilationError(actual ExpectedTaskResponse) bool {
	return actual.Payload.StatusCode == int(solution.CompilationError) &&
		strings.Contains(actual.Payload.Message, "exit status") &&
		actual.Payload.TestResults == nil
}

func isTestCaseFailed(actual ExpectedTaskResponse) bool {
	return actual.Payload.StatusCode == int(solution.TestFailed) &&
		strings.Contains(actual.Payload.Message, constants.SolutionMessageTestFailed) &&
		len(actual.Payload.TestResults) > 0 && !actual.Payload.TestResults[0].Passed
}

var validators = map[tests.TestType]func(ExpectedTaskResponse) bool{
	tests.CPPSuccess:                 isSuccess,
	tests.CPPFailedTimeLimitExceeded: isTimeLimitExceeded,
	tests.CPPCompilationError:        isCompilationError,
	tests.CPPTestCaseFailed:          isTestCaseFailed,
}

var testCases = []struct {
	name            string
	testType        tests.TestType
	languageType    string
	languageVersion string
}{
	{"Test valid solution CPP", tests.CPPSuccess, "CPP", "20"},
	{"Test solution with time limit exceeded CPP", tests.CPPFailedTimeLimitExceeded, "CPP", "20"},
	{"Test solution with compilation error CPP", tests.CPPCompilationError, "CPP", "20"},
	{"Test solution with test case failed CPP", tests.CPPTestCaseFailed, "CPP", "20"},
}

func validateResponse(testType tests.TestType, actual ExpectedTaskResponse) bool {
	validator, ok := validators[testType]
	if !ok {
		return false
	}
	return validator(actual)
}

func validateErrFileContent(testType tests.TestType, outputDir string) bool {
	switch testType {
	case tests.CPPSuccess:
		return fileExists(outputDir, "1.err") && fileContains(outputDir, "1.err", "")
	case tests.CPPFailedTimeLimitExceeded:
		return fileExists(outputDir, "1.err") && fileContains(outputDir, "1.err", "timeout")
	case tests.CPPCompilationError:
		return fileExists(outputDir, "compile-err.err") && fileContains(outputDir, "compile-err.err", "errors")
	case tests.CPPTestCaseFailed:
		return fileExists(outputDir, "1.err") && fileContains(outputDir, "1.err", "1c1")
	case tests.Handshake: // to make revive linter happy
		return false
	case tests.LongTaskMessage:
		return false
	case tests.Status:
		return false
	default:
		return false
	}
}

func fileExists(dir, filename string) bool {
	_, err := os.Stat(fmt.Sprintf("%s/%s", dir, filename))
	return err == nil
}

func fileContains(dir, filename, content string) bool {
	file, err := os.ReadFile(fmt.Sprintf("%s/%s", dir, filename))
	if err != nil {
		return false
	}

	return strings.Contains(string(file), content)
}

func equalHandshskePayload(actualResponse []LanguageConfig, expectedPayload []LanguageConfig) bool {
	if len(actualResponse) != len(expectedPayload) {
		return false
	}

	for _, lang := range actualResponse {
		foundLang := false
		for _, expectedLang := range expectedPayload {
			if lang.Name == expectedLang.Name {
				foundLang = true
				if len(lang.Versions) != len(expectedLang.Versions) {
					return false
				}

				for _, version := range lang.Versions {
					foundVersion := false
					for _, expectedVersion := range expectedLang.Versions {
						if version == expectedVersion {
							foundVersion = true
							break
						}
					}

					if !foundVersion {
						return false
					}
				}
			}
		}
		if !foundLang {
			return false
		}
	}

	return true
}

func setUp(t *testing.T, numberOfWorkers int) (services.QueueService, *amqp.Channel, *amqp.Connection) {
	fs := tests.NewMockFileService(t)
	rs := services.NewRunnerService()

	config := config.NewConfig()
	conn := rabbitmq.NewRabbitMqConnection(config)
	channel := rabbitmq.NewRabbitMQChannel(conn)

	wp := services.NewWorkerPool(channel, constants.DefaultWorkerQueueName, numberOfWorkers, fs, rs)
	qs := services.NewQueueService(channel, constants.DefaultWorkerQueueName, wp)

	if _, err := os.Stat(tests.MockTmpDir); os.IsNotExist(err) {
		err := os.Mkdir(tests.MockTmpDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create tmp directory: %s", err)
		}
	}

	return qs, channel, conn
}

func tearDown(t *testing.T) {
	// remove temporary directory
	err := os.RemoveAll(tests.MockTmpDir)
	if err != nil {
		t.Errorf("Failed to remove temporary directory: %s", err)
	}

	// remove logs directory
	err = os.RemoveAll("./internal/logger/logs")
	if err != nil {
		t.Errorf("Failed to remove logs directory: %s", err)
	}
}

func TestProcessTask(t *testing.T) {
	qs, channel, conn := setUp(t, 1)
	defer conn.Close()
	defer channel.Close()
	defer tearDown(t)

	go qs.Listen()

	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			message := generateQueueMessage(tt.testType, tt.languageType, tt.languageVersion)
			errChan := make(chan error, 1)
			go func() {
				errChan <- publishMessage(channel, message)
			}()

			if err := <-errChan; err != nil {
				t.Fatalf("Failed to publish message: %s", err)
			}

			select {
			case response := <-responseChannel:
				var actualResponse ExpectedTaskResponse
				err := json.Unmarshal(response.Body, &actualResponse)
				if err != nil {
					t.Fatalf("Failed to parse response JSON: %s", err)
				}

				t.Logf("Response: %+v", actualResponse)

				if !validateResponse(tt.testType, actualResponse) {
					t.Fatalf("Unexpected response: %+v", actualResponse)
				}

				var outputDir string
				if tt.testType == tests.CPPCompilationError {
					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d", tt.testType)
				} else {
					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d/%s", tt.testType, actualResponse.Payload.OutputDir)
				}

				if !validateErrFileContent(tt.testType, outputDir) {
					t.Fatalf("Unexpected error file content")
				}

			case <-time.After(5 * time.Second):
				t.Fatalf("Did not receive response in time")
			}
		})
	}
}
func TestProcessHandshake(t *testing.T) {
	qs, channel, conn := setUp(t, 1)
	defer conn.Close()
	defer channel.Close()
	defer tearDown(t)

	go qs.Listen()

	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	t.Run("Test handshake", func(t *testing.T) {
		message := generateQueueMessage(tests.Handshake, "", "")
		errChan := make(chan error, 1)
		go func() {
			errChan <- publishMessage(channel, message)
		}()

		if err := <-errChan; err != nil {
			t.Fatalf("Failed to publish message: %s", err)
		}

		select {
		case response := <-responseChannel:
			var actualResponse ExpectedHandshakeResponse
			err := json.Unmarshal(response.Body, &actualResponse)
			if err != nil {
				t.Fatalf("Failed to parse response JSON: %s", err)
			}

			if actualResponse.Type != "handshake" {
				t.Fatalf("Unexpected response type: %s", actualResponse.Type)
			}

			expectedPayload := []LanguageConfig{
				{
					Name:     "CPP",
					Versions: []string{"20", "17", "14", "11"},
				},
			}

			if !equalHandshskePayload(actualResponse.Payload.Languages, expectedPayload) {
				t.Fatalf("Unexpected response payload: %+v", actualResponse.Payload.Languages)
			}

		case <-time.After(5 * time.Second):
			t.Fatalf("Did not receive response in time")
		}
	})
}

func TestProcessStatus(t *testing.T) {
	const numberOfWorkers = 5
	qs, channel, conn := setUp(t, numberOfWorkers)
	defer conn.Close()
	defer channel.Close()
	defer tearDown(t)

	go qs.Listen()

	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	t.Run("Test status all idle", func(t *testing.T) {
		testAllIdle(t, channel, responseChannel, numberOfWorkers)
	})

	t.Run("Test 1 busy worker", func(t *testing.T) {
		testOneBusyWorker(t, channel, responseChannel, numberOfWorkers)
	})
}

func testAllIdle(t *testing.T, channel *amqp.Channel, responseChannel <-chan amqp.Delivery, numberOfWorkers int) {
	message := generateQueueMessage(tests.Status, "", "")
	publishAsync(t, channel, message)

	select {
	case response := <-responseChannel:
		var actual ExpecredStatusResponse
		require.NoError(t, json.Unmarshal(response.Body, &actual))
		require.Equal(t, "status", actual.Type)
		require.Equal(t, 0, actual.Payload.BusyWorkers)
		require.Len(t, actual.Payload.WorkerStatus, numberOfWorkers)

		for _, status := range actual.Payload.WorkerStatus {
			require.Equal(t, "idle", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive response in time")
	}
}

func testOneBusyWorker(t *testing.T, channel *amqp.Channel, responseChannel <-chan amqp.Delivery, numberOfWorkers int) {
	message := generateQueueMessage(tests.LongTaskMessage, "CPP", "20")
	publishAsync(t, channel, message)
	time.Sleep(3 * time.Second)

	statusMessage := generateQueueMessage(tests.Status, "", "")
	publishAsync(t, channel, statusMessage)

	select {
	case response := <-responseChannel:
		var actual ExpecredStatusResponse
		require.NoError(t, json.Unmarshal(response.Body, &actual))
		require.Equal(t, 1, actual.Payload.BusyWorkers)
		require.Len(t, actual.Payload.WorkerStatus, numberOfWorkers)

		busy := 0
		for _, status := range actual.Payload.WorkerStatus {
			if strings.Contains(status, "busy") {
				busy++
			}
		}
		require.Equal(t, 1, busy)
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive response in time")
	}
}

func publishAsync(t *testing.T, channel *amqp.Channel, msg []byte) {
	errChan := make(chan error, 1)
	go func() {
		errChan <- publishMessage(channel, msg)
	}()
	require.NoError(t, <-errChan)
}
