package tests

// import (
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"strings"
// 	"testing"
// 	"time"

// 	"github.com/mini-maxit/worker/internal/config"
// 	"github.com/mini-maxit/worker/internal/constants"
// 	"github.com/mini-maxit/worker/internal/services"
// 	"github.com/mini-maxit/worker/internal/solution"
// 	"github.com/mini-maxit/worker/rabbitmq"
// 	"github.com/mini-maxit/worker/utils"
// 	amqp "github.com/rabbitmq/amqp091-go"
// )

// type testType int

// const (
// 	Success testType = iota + 1
// 	FailedTimeLimitExceeded
// 	CompilationError
// 	TestCaseFailed
// 	Handshake
// )

// // Struct for validating response payload
// type ExpectedTaskResponse struct {
// 	Type      string `json:"type"`
// 	MessageID string `json:"message_id"`
// 	Payload   struct {
// 		OutputDir   string                `json:"OutputDir"`
// 		Success     bool                  `json:"Success"`
// 		StatusCode  int                   `json:"StatusCode"`
// 		Code        string                `json:"Code"`
// 		Message     string                `json:"Message"`
// 		TestResults []solution.TestResult `json:"TestResults"`
// 	} `json:"payload"`
// }

// type ExpectedHandshakeResponse struct {
// 	Type      string          `json:"type"`
// 	MessageID string          `json:"message_id"`
// 	Payload   json.RawMessage `json:"payload"`
// }

// func generateQueueMessage(test testType) string {
// 	var payload string
// 	var msgType string

// 	if test == Handshake {
// 		payload = "{}"
// 		msgType = "handshake"
// 	} else {
// 		payload = fmt.Sprintf(`{"task_id":1,"user_id":1,"submission_number":%d,"language_type":"CPP","language_version":"20","time_limits":[2],"memory_limits":[512],"chroot_dir_path":"./mock_files/tmp","use_chroot":"false"}`, test)
// 		msgType = "task"
// 	}

// 	return fmt.Sprintf(`{"type":"%s","message_id":"adsa","payload":%s}`, msgType, payload)
// }

// func declareResponseQueue(ch *amqp.Channel, queueName string) (amqp.Queue, error) {
// 	return ch.QueueDeclare(queueName, false, false, false, false, nil)
// }

// func consumeResponse(ch *amqp.Channel, queueName string) (<-chan amqp.Delivery, error) {
// 	return ch.Consume(queueName, "", true, false, false, false, nil)
// }

// func publishMessage(ch *amqp.Channel, message string) error {
// 	return ch.Publish(
// 		"",             // exchange
// 		"worker_queue", // routing key
// 		false,          // mandatory
// 		false,          // immediate
// 		amqp.Publishing{
// 			ContentType: "application/json",
// 			Body:        []byte(message),
// 			ReplyTo:     "reply_to",
// 		})
// }

// func validateResponse(testType testType, actual ExpectedTaskResponse) bool {
// 	switch testType {
// 	case Success:
// 		return actual.Payload.Success &&
// 			actual.Payload.Code == solution.Success.String() &&
// 			strings.Contains(actual.Payload.Message, "solution executed successfully") &&
// 			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 && (actual.Payload.TestResults)[0].Passed
// 	case FailedTimeLimitExceeded:
// 		return !actual.Payload.Success &&
// 			actual.Payload.Code == solution.RuntimeError.String() &&
// 			strings.Contains(actual.Payload.Message, "time limit exceeded") &&
// 			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) == 1 &&
// 			!(actual.Payload.TestResults)[0].Passed && (actual.Payload.TestResults)[0].ErrorMessage == "time limit exceeded"
// 	case CompilationError:
// 		return !actual.Payload.Success &&
// 			actual.Payload.Code == solution.CompilationError.String() &&
// 			strings.Contains(actual.Payload.Message, "exit status") &&
// 			actual.Payload.TestResults == nil
// 	case TestCaseFailed:
// 		return !actual.Payload.Success &&
// 			actual.Payload.Code == solution.Success.String() &&
// 			strings.Contains(actual.Payload.Message, "solution executed successfully") &&
// 			actual.Payload.TestResults != nil && len(actual.Payload.TestResults) > 0 &&
// 			!(actual.Payload.TestResults)[0].Passed &&
// 			strings.Contains((actual.Payload.TestResults)[0].ErrorMessage, "Difference at line 1")
// 	default:
// 		return false
// 	}
// }

// func validateErrFileContent(testType testType, outputDir string) bool {
// 	switch testType {
// 	case Success:
// 		return true
// 	case FailedTimeLimitExceeded:
// 		return fileExists(outputDir, "1.err") && fileContains(outputDir, "1.err", "timeout")
// 	case CompilationError:
// 		return fileExists(outputDir, "compile-err.err") && fileContains(outputDir, "compile-err.err", "undeclared identifier 'std'")
// 	case TestCaseFailed:
// 		return fileExists(outputDir, "1.err") && fileContains(outputDir, "1.err", "Difference at line 1")
// 	default:
// 		return false
// 	}
// }

// func fileExists(dir, filename string) bool {
// 	_, err := os.Stat(fmt.Sprintf("%s/%s", dir, filename))
// 	return err == nil
// }

// func fileContains(dir, filename, content string) bool {
// 	file, err := os.ReadFile(fmt.Sprintf("%s/%s", dir, filename))
// 	if err != nil {
// 		return false
// 	}

// 	return strings.Contains(string(file), content)
// }

// func equalHandshskePayload(payload map[string][]string, expectedPayload map[string][]string) bool {
// 	if len(payload) != len(expectedPayload) {
// 		return false
// 	}

// 	for k, v := range payload {
// 		if _, ok := expectedPayload[k]; !ok {
// 			return false
// 		}

// 		for _, version := range v {
// 			if !utils.Contains(expectedPayload[k], version) {
// 				return false
// 			}
// 		}
// 	}

// 	return true
// }

// func setUp(t *testing.T) (services.QueueService, *amqp.Channel, *amqp.Connection) {
// 	fs := NewMockFileService(t)
// 	rs := services.NewRunnerService()
// 	w := services.NewWorker(fs, rs)

// 	config := config.NewConfig()
// 	conn := rabbitmq.NewRabbitMqConnection(config)
// 	channel := rabbitmq.NewRabbitMQChannel(conn)
// 	qs := services.NewQueueService(channel, constants.WorkerQueueName, w)
// 	return qs, channel, conn
// }

// func TestProcessTask(t *testing.T) {
// 	qs, channel, conn := setUp(t)
// 	defer conn.Close()
// 	defer channel.Close()

// 	go qs.Listen()

// 	responseQueueName := "reply_to"
// 	_, err := declareResponseQueue(channel, responseQueueName)
// 	if err != nil {
// 		t.Fatalf("Failed to declare response queue: %s", err)
// 	}
// 	responseChannel, err := consumeResponse(channel, responseQueueName)
// 	if err != nil {
// 		t.Fatalf("Failed to consume response queue: %s", err)
// 	}

// 	tests := []struct {
// 		name     string
// 		testType testType
// 	}{
// 		{"Test valid solution", Success},
// 		{"Test solution with time limit exceeded", FailedTimeLimitExceeded},
// 		{"Test solution with compilation error", CompilationError},
// 		{"Test solution with test case failed", TestCaseFailed},
// 	}

// 	for _, tt := range tests {
// 		taskDir := fmt.Sprintf("./mock_files/tmp/Task_1_1_%d", tt.testType)
// 		t.Run(tt.name, func(t *testing.T) {
// 			message := generateQueueMessage(tt.testType)
// 			go publishMessage(channel, message)

// 			select {
// 			case response := <-responseChannel:
// 				var actualResponse ExpectedTaskResponse
// 				err := json.Unmarshal(response.Body, &actualResponse)
// 				if err != nil {
// 					err = os.RemoveAll(taskDir)
// 					if err != nil {
// 						t.Fatalf("Failed to remove task directory: %s", err)
// 					}
// 					t.Fatalf("Failed to parse response JSON: %s", err)
// 				}

// 				if !validateResponse(tt.testType, actualResponse) {
// 					err = os.RemoveAll(taskDir)
// 					if err != nil {
// 						t.Fatalf("Failed to remove task directory: %s", err)
// 					}
// 					t.Fatalf("Unexpected response: %+v", actualResponse)
// 				}

// 				var outputDir string
// 				if tt.testType == CompilationError {
// 					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d", tt.testType)
// 				} else {
// 					outputDir = fmt.Sprintf("./mock_files/tmp/Task_1_1_%d/%s", tt.testType, actualResponse.Payload.OutputDir)
// 				}

// 				if !validateErrFileContent(tt.testType, outputDir) {
// 					err = os.RemoveAll(taskDir)
// 					if err != nil {
// 						t.Fatalf("Failed to remove task directory: %s", err)
// 					}
// 					t.Fatalf("Unexpected error file content")
// 				}

// 				if err := os.RemoveAll(taskDir); err != nil {
// 					t.Fatalf("Failed to remove task directory: %s", err)
// 				}

// 			case <-time.After(5 * time.Second):
// 				err = os.RemoveAll(taskDir)
// 				if err != nil {
// 					t.Fatalf("Failed to remove task directory: %s", err)
// 				}
// 				t.Fatalf("Did not receive response in time")
// 			}
// 		})
// 	}
// }

// func TestProcessHandshake(t *testing.T) {
// 	qs, channel, conn := setUp(t)
// 	defer conn.Close()
// 	defer channel.Close()

// 	go qs.Listen()

// 	responseQueueName := "reply_to"
// 	_, err := declareResponseQueue(channel, responseQueueName)
// 	if err != nil {
// 		t.Fatalf("Failed to declare response queue: %s", err)
// 	}
// 	responseChannel, err := consumeResponse(channel, responseQueueName)
// 	if err != nil {
// 		t.Fatalf("Failed to consume response queue: %s", err)
// 	}

// 	t.Run("Test handshake", func(t *testing.T) {
// 		message := generateQueueMessage(Handshake)
// 		go publishMessage(channel, message)

// 		select {
// 		case response := <-responseChannel:
// 			var actualResponse ExpectedHandshakeResponse
// 			err := json.Unmarshal(response.Body, &actualResponse)
// 			if err != nil {
// 				t.Fatalf("Failed to parse response JSON: %s", err)
// 			}

// 			if actualResponse.Type != "handshake" {
// 				t.Fatalf("Unexpected response type: %s", actualResponse.Type)
// 			}

// 			var payload map[string][]string
// 			err = json.Unmarshal(actualResponse.Payload, &payload)
// 			if err != nil {
// 				t.Fatalf("Failed to parse response payload JSON: %s", err)
// 			}

// 			expectedPayload := map[string][]string{
// 				"CPP": {"11", "14", "17", "20"},
// 			}

// 			if !equalHandshskePayload(payload, expectedPayload) {
// 				t.Fatalf("Unexpected response payload: %+v", payload)
// 			}

// 		case <-time.After(5 * time.Second):
// 			t.Fatalf("Did not receive response in time")
// 		}
// 	})
// }
