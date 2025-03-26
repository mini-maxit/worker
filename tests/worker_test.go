package tests

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
	amqp "github.com/rabbitmq/amqp091-go"
)

type testType int

const (
	CPPSuccess testType = iota + 1
	CPPFailedTimeLimitExceeded
	CPPCompilationError
	CPPTestCaseFailed
	PythonSuccess
	PythonFailedTimeLimitExceeded
	PythonTestCaseFailed
	Handshake
	longTaskMessage
	Status
)

// Struct for validating response payload
type ExpectedTaskResponse struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	Payload   struct {
		OutputDir   string                `json:"OutputDir"`
		StatusCode  int                   `json:"StatusCode"`
		Code        string                `json:"Code"`
		Message     string                `json:"Message"`
		TestResults []solution.TestResult `json:"TestResults"`
	} `json:"payload"`
}

type ExpectedHandshakeResponse struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	Payload   struct {
		Languages []LanguageConfig `json:"languages"`
	} `json:"payload"`
}

type LanguageConfig struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type ExpecredStatusResponse struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	Payload   struct {
		BusyWorkers  int               `json:"busy_workers"`
		TotalWorkers int               `json:"total_workers"`
		WorkerStatus map[string]string `json:"worker_status"`
	} `json:"payload"`
}

func generateQueueMessage(test testType) []byte {
	var payload map[string]interface{}
	var msgType string

	if test == Handshake {
		payload = map[string]interface{}{}
		msgType = "handshake"
	} else if test == Status {
		payload = map[string]interface{}{}
		msgType = "status"
	} else if test == longTaskMessage {
		payload = map[string]interface{}{
			"task_id":           1,
			"user_id":           1,
			"submission_number": FailedTimeLimitExceeded,
			"language_type":     "CPP",
			"language_version":  "20",
			"time_limits":       []int{20},
			"memory_limits":     []int{512},
			"chroot_dir_path":   fmt.Sprintf("%s/Task_1_1_%d", mockTmpDir, FailedTimeLimitExceeded),
			"use_chroot":        "false",
		}
		msgType = "task"
	} else {
		payload = map[string]interface{}{
			"task_id":           1,
			"user_id":           1,
			"submission_number": test,
			"language_type":     "CPP",
			"language_version":  "20",
			"time_limits":       []int{2},
			"memory_limits":     []int{512},
			"chroot_dir_path":   fmt.Sprintf("%s/Task_1_1_%d", mockTmpDir, test),
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
			ReplyTo:     "reply_to",
		})
}

func validateResponse(testType testType, actual ExpectedTaskResponse) bool {
	switch testType {
	case CPPSuccess:
		return actual.Payload.StatusCode == int(solution.Success) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageSuccess) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 && (actual.Payload.TestResults)[0].Passed
	case CPPFailedTimeLimitExceeded:
		return actual.Payload.StatusCode == int(solution.TimeLimitExceeded) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageTimeout) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 &&
			!(actual.Payload.TestResults)[0].Passed && (actual.Payload.TestResults)[0].ErrorMessage == constants.TestMessageTimeLimitExceeded
	case CPPCompilationError:
		return actual.Payload.StatusCode == int(solution.CompilationError) &&
			strings.Contains(actual.Payload.Message, "exit status") &&
			actual.Payload.TestResults == nil
	case CPPTestCaseFailed:
		return actual.Payload.StatusCode == int(solution.TestFailed) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageTestFailed) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) > 0 &&
			!(actual.Payload.TestResults)[0].Passed
	case PythonSuccess:
		return actual.Payload.StatusCode == int(solution.Success) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageSuccess) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 && (actual.Payload.TestResults)[0].Passed
	case PythonFailedTimeLimitExceeded:
		return actual.Payload.StatusCode == int(solution.TimeLimitExceeded) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageTimeout) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 &&
			!(actual.Payload.TestResults)[0].Passed && (actual.Payload.TestResults)[0].ErrorMessage == constants.TestMessageTimeLimitExceeded
	case PythonTestCaseFailed:
		return actual.Payload.StatusCode == int(solution.TestFailed) &&
			strings.Contains(actual.Payload.Message, constants.SolutionMessageTestFailed) &&
			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) > 0 &&
			!(actual.Payload.TestResults)[0].Passed
	default:
		return false
	}
}

func validateErrFileContent(testType testType, outputDir string) bool {
	switch testType {
	case CPPSuccess:
		return fileExists(outputDir, "1.err")
	case CPPFailedTimeLimitExceeded:
		return fileExists(outputDir, "1.err")
	case CPPCompilationError:
		return fileExists(outputDir, "compile-err.err")
	case CPPTestCaseFailed:
		return fileExists(outputDir, "1.err")
	case PythonSuccess:
		return fileExists(outputDir, "1.err")
	case PythonFailedTimeLimitExceeded:
		return fileExists(outputDir, "1.err")
	case PythonTestCaseFailed:
		return fileExists(outputDir, "1.err")
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
	fs := NewMockFileService(t)
	rs := services.NewRunnerService()

	config := config.NewConfig()
	conn := rabbitmq.NewRabbitMqConnection(config)
	channel := rabbitmq.NewRabbitMQChannel(conn)

	wp := services.NewWorkerPool(channel, constants.DefaultWorkerQueueName, numberOfWorkers, fs, rs)
	qs := services.NewQueueService(channel, constants.DefaultWorkerQueueName, wp)

	if _, err := os.Stat(mockTmpDir); os.IsNotExist(err) {
		err := os.Mkdir(mockTmpDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create tmp directory: %s", err)
		}
	}

	return qs, channel, conn
}

func TestProcessTask(t *testing.T) {
	qs, channel, conn := setUp(t, 1)
	defer conn.Close()
	defer channel.Close()

	go qs.Listen()

	responseQueueName := "reply_to"
	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	tests := []struct {
		name            string
		testType        testType
		languageType    string
		languageVersion string
	}{
		{"Test valid solution CPP", CPPSuccess, "CPP", "20"},
		{"Test solution with time limit exceeded CPP", CPPFailedTimeLimitExceeded, "CPP", "20"},
		{"Test solution with compilation error CPP", CPPCompilationError, "CPP", "20"},
		{"Test solution with test case failed CPP", CPPTestCaseFailed, "CPP", "20"},
		{"Test valid solution Python", PythonSuccess, "PYTHON", "3"},
		{"Test solution with time limit exceeded Python", PythonFailedTimeLimitExceeded, "PYTHON", "3"},
		{"Test solution with test case failed Python", PythonTestCaseFailed, "PYTHON", "3"},
	}

	for _, tt := range tests {
		taskDir := fmt.Sprintf("./mock_files/tmp/Task_1_1_%d", tt.testType)
		t.Run(tt.name, func(t *testing.T) {
			message := generateQueueMessage(tt.testType, tt.languageType, tt.languageVersion)
			go publishMessage(channel, message)

			select {
			case response := <-responseChannel:
				var actualResponse ExpectedTaskResponse
				err := json.Unmarshal(response.Body, &actualResponse)
				if err != nil {
					err = os.RemoveAll(taskDir)
					if err != nil {
						t.Fatalf("Failed to remove task directory: %s", err)
					}
					t.Fatalf("Failed to parse response JSON: %s", err)
				}

				if !validateResponse(tt.testType, actualResponse) {
					err = os.RemoveAll(taskDir)
					if err != nil {
						t.Fatalf("Failed to remove task directory: %s", err)
					}
					t.Fatalf("Unexpected response: %+v", actualResponse)
				}

				var outputDir string
				if tt.testType == CPPCompilationError {
					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d", tt.testType)
				} else {
					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d/%s", tt.testType, actualResponse.Payload.OutputDir)
				}

				if !validateErrFileContent(tt.testType, outputDir) {
					err = os.RemoveAll(taskDir)
					if err != nil {
						t.Fatalf("Failed to remove task directory: %s", err)
					}
					t.Fatalf("Unexpected error file content")
				}

				if err := os.RemoveAll(taskDir); err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}

			case <-time.After(5 * time.Second):
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
				t.Fatalf("Did not receive response in time")
			}
		})
	}

	err = os.RemoveAll(mockTmpDir)
	if err != nil {
		t.Fatalf("Failed to remove tmp directory: %s", err)
	}
}
func TestProcessHandshake(t *testing.T) {
	qs, channel, conn := setUp(t, 1)
	defer conn.Close()
	defer channel.Close()

	go qs.Listen()

	responseQueueName := "reply_to"
	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	t.Run("Test handshake", func(t *testing.T) {
		message := generateQueueMessage(Handshake, "", "")
		go publishMessage(channel, message)

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

	err = os.RemoveAll(mockTmpDir)
	if err != nil {
		t.Fatalf("Failed to remove tmp directory: %s", err)
	}
}

func TestProcessStatus(t *testing.T) {
	const numberOfWorkers = 5
	const taskDir = "./mock_files/tmp/Task_1_1_2"

	qs, channel, conn := setUp(t, numberOfWorkers)
	defer conn.Close()
	defer channel.Close()

	go qs.Listen()

	responseQueueName := "reply_to"
	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	t.Run("Test status all idle", func(t *testing.T) {
		message := generateQueueMessage(Status)
		go publishMessage(channel, message)

		select {
		case response := <-responseChannel:
			var actualResponse ExpecredStatusResponse
			err := json.Unmarshal(response.Body, &actualResponse)
			if err != nil {
				t.Fatalf("Failed to parse response JSON: %s", err)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Type != "status" {
				t.Fatalf("Unexpected response type: %s", actualResponse.Type)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Payload.BusyWorkers != 0 {
				t.Fatalf("Unexpected busy workers count: %d", actualResponse.Payload.BusyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if len(actualResponse.Payload.WorkerStatus) != numberOfWorkers {
				t.Fatalf("Unexpected worker status count: %d", len(actualResponse.Payload.WorkerStatus))
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			for _, status := range actualResponse.Payload.WorkerStatus {
				if status != "idle" {
					t.Fatalf("Unexpected worker status: %s", status)
					err = os.RemoveAll(taskDir)
					if err != nil {
						t.Fatalf("Failed to remove task directory: %s", err)
					}
				}
			}

		case <-time.After(5 * time.Second):
			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}
			t.Fatalf("Did not receive response in time")
		}
	})

	t.Run("Test 1 busy worker", func(t *testing.T) {
		message := generateQueueMessage(longTaskMessage)
		go publishMessage(channel, message)

		time.Sleep(3 * time.Second)

		message = generateQueueMessage(Status)
		go publishMessage(channel, message)

			expectedPayload := map[string][]string{
				"CPP":    {"11", "14", "17", "20"},
				"PYTHON": {"2", "3"},
			}

			if actualResponse.Payload.BusyWorkers != 1 {
				t.Fatalf("Unexpected busy workers count: %d", actualResponse.Payload.BusyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if len(actualResponse.Payload.WorkerStatus) != numberOfWorkers {
				t.Fatalf("Unexpected worker status count: %d", len(actualResponse.Payload.WorkerStatus))
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			busyWorkers := 0
			for _, status := range actualResponse.Payload.WorkerStatus {
				if strings.Contains(status, "busy") {
					busyWorkers++
				}
			}

			if busyWorkers != 1 {
				t.Fatalf("Unexpected busy workers count: %d", busyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}

		case <-time.After(5 * time.Second):
			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}
			t.Fatalf("Did not receive response in time")
		}
	})

	err = os.RemoveAll(mockTmpDir)
	if err != nil {
		t.Fatalf("Failed to remove tmp directory: %s", err)
	}
}

func TestProcessStatus(t *testing.T) {
	const numberOfWorkers = 5
	const taskDir = "./mock_files/tmp/Task_1_1_2"

	qs, channel, conn := setUp(t, numberOfWorkers)
	defer conn.Close()
	defer channel.Close()

	go qs.Listen()

	responseQueueName := "reply_to"
	_, err := declareResponseQueue(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to declare response queue: %s", err)
	}
	responseChannel, err := consumeResponse(channel, responseQueueName)
	if err != nil {
		t.Fatalf("Failed to consume response queue: %s", err)
	}

	t.Run("Test status all idle", func(t *testing.T) {
		message := generateQueueMessage(Status, "", "")
		go publishMessage(channel, message)

		select {
		case response := <-responseChannel:
			var actualResponse ExpecredStatusResponse
			err := json.Unmarshal(response.Body, &actualResponse)
			if err != nil {
				t.Fatalf("Failed to parse response JSON: %s", err)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Type != "status" {
				t.Fatalf("Unexpected response type: %s", actualResponse.Type)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Payload.BusyWorkers != 0 {
				t.Fatalf("Unexpected busy workers count: %d", actualResponse.Payload.BusyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if len(actualResponse.Payload.WorkerStatus) != numberOfWorkers {
				t.Fatalf("Unexpected worker status count: %d", len(actualResponse.Payload.WorkerStatus))
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			for _, status := range actualResponse.Payload.WorkerStatus {
				if status != "idle" {
					t.Fatalf("Unexpected worker status: %s", status)
					err = os.RemoveAll(taskDir)
					if err != nil {
						t.Fatalf("Failed to remove task directory: %s", err)
					}
				}
			}

		case <-time.After(5 * time.Second):
			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}
			t.Fatalf("Did not receive response in time")
		}
	})

	t.Run("Test 1 busy worker", func(t *testing.T) {
		message := generateQueueMessage(longTaskMessage, "CPP", "20")
		go publishMessage(channel, message)

		time.Sleep(3 * time.Second)

		message = generateQueueMessage(Status, "", "")
		go publishMessage(channel, message)

		select {
		case response := <-responseChannel:
			var actualResponse ExpecredStatusResponse
			err := json.Unmarshal(response.Body, &actualResponse)
			if err != nil {
				t.Fatalf("Failed to parse response JSON: %s", err)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Type != "status" {
				t.Fatalf("Unexpected response type: %s", actualResponse.Type)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if actualResponse.Payload.BusyWorkers != 1 {
				t.Fatalf("Unexpected busy workers count: %d", actualResponse.Payload.BusyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			if len(actualResponse.Payload.WorkerStatus) != numberOfWorkers {
				t.Fatalf("Unexpected worker status count: %d", len(actualResponse.Payload.WorkerStatus))
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			busyWorkers := 0
			for _, status := range actualResponse.Payload.WorkerStatus {
				if strings.Contains(status, "busy") {
					busyWorkers++
				}
			}

			if busyWorkers != 1 {
				t.Fatalf("Unexpected busy workers count: %d", busyWorkers)
				err = os.RemoveAll(taskDir)
				if err != nil {
					t.Fatalf("Failed to remove task directory: %s", err)
				}
			}

			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}

		case <-time.After(5 * time.Second):
			err = os.RemoveAll(taskDir)
			if err != nil {
				t.Fatalf("Failed to remove task directory: %s", err)
			}
			t.Fatalf("Did not receive response in time")
		}
	})
}
