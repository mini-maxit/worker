package tests

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/worker"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap/zapcore"
)

const validMessage = `{
  "message_id": "adsa",
  "task_id": 1,
  "user_id": 1,
  "submission_number": 1,
  "language_type": "CPP",
  "language_version": "20",
  "time_limits": [10],
  "memory_limits": [512]
}`

type MessageType int

const (
	Success MessageType = iota + 1
	CompilationError
	FailedTimeLimitExceeded
)

type workerTestStruct struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	queueName  string
}

func setup() *workerTestStruct {
	config := config.NewConfig()
	conn := worker.NewRabbitMqConnection(config)

	ch := worker.NewRabbitMQChannel(conn)

	_, err := declareResponseQueue(ch, "reply_to")
	if err != nil {
		return &workerTestStruct{
			connection: nil,
			channel:    nil,
			queueName:  "",
		}
	}

	return &workerTestStruct{
		connection: conn,
		channel:    ch,
		queueName:  "reply_to",
	}
}

func tearDown(worker *workerTestStruct) {
	worker.channel.Close()
	worker.connection.Close()
}

func createTask(taskPath string) error {
	requestUrl := "http://file-storage:8888/createTask"
	file, err := os.Open(taskPath)
	if err != nil {
		return err
	}

	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("taskID", "1"); err != nil {
		return err
	}
	if err := writer.WriteField("overwrite", "true"); err != nil {
		return err
	}

	part, err := writer.CreateFormFile("archive", filepath.Base(taskPath))
	if err != nil {
		return err
	}

	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", requestUrl, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		_, err := bodyBytes.ReadFrom(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to create task: %s", bodyBytes.String())
	}

	return nil
}

func declareResponseQueue(ch *amqp.Channel, queueName string) (amqp.Queue, error) {
	return ch.QueueDeclare(queueName, false, false, false, false, nil)
}

func consumeResponse(ch *amqp.Channel, queueName string) (<-chan amqp.Delivery, error) {
	return ch.Consume(queueName, "", true, false, false, false, nil)
}

func publishTask(ch *amqp.Channel) error {
	err := ch.Publish(
		"",             // exchange
		"worker_queue", // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(validMessage),
			Headers: amqp.Table{
				"x-retry-count": 1,
			},
			ReplyTo: "reply_to",
		})
	if err != nil {
		return err
	}

	return nil
}

func submitSubmission(msgType MessageType, taskID, userID string) error {
	requestUrl := "http://file-storage:8888/submit"
	filePath := "../tests/mock_files/"
	switch msgType {
	case Success:
		filePath += "under_limit.cpp"
	case FailedTimeLimitExceeded:
		filePath += "over_limit.cpp"
	case CompilationError:
		filePath += "compilation_error.cpp"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("taskID", taskID); err != nil {
		return err
	}

	if err := writer.WriteField("userID", userID); err != nil {
		return err
	}

	part, err := writer.CreateFormFile("submissionFile", filepath.Base(filePath))
	if err != nil {
		return err
	}

	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", requestUrl, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		_, err := bodyBytes.ReadFrom(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to submit submission: %s", bodyBytes.String())
	}

	return nil
}

func deleteTask(taskID string) error {
	requestUrl := fmt.Sprintf("http://file-storage:8888/deleteTask?taskID=%s", taskID)
	req, err := http.NewRequest("DELETE", requestUrl, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		_, err := bodyBytes.ReadFrom(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to delete task: %s", bodyBytes.String())
	}

	return nil
}

func generateExpectedResponseMessage(msgType MessageType) string {
	var expectedResponse strings.Builder
	var statusCode solution.SolutionStatus
	var message string
	var code string
	var sucess bool
	var testResult string

	switch msgType {
	case Success:
		code = solution.Success.String()
		statusCode = solution.Success
		message = "solution executed successfully"
		sucess = true
		testResult = `"TestResults":[{"Passed":true,"ErrorMessage":"","Order":1}]`
	case FailedTimeLimitExceeded:
		code = solution.RuntimeError.String()
		message = "some test cases failed due to time limit exceeded"
		statusCode = solution.RuntimeError
		sucess = false
		testResult = `"TestResults":[{"Passed":false,"ErrorMessage":"time limit exceeded","Order":1}]`
	case CompilationError:
		code = solution.CompilationError.String()
		statusCode = solution.CompilationError
		message = "exit status 1"
		sucess = false
		testResult = `"TestResults":null`
	default:
		code = "Unknown"
	}

	message = fmt.Sprintf(`{"message_id":"adsa","result":{"OutputDir":"user-output","Success":%t,"StatusCode":%d,"Code":"%s","Message":"%s",%s}}`, sucess, statusCode, code, message, testResult)

	expectedResponse.WriteString(message)

	return expectedResponse.String()
}

func TestValidExecution(t *testing.T) {
	worker := setup()
	testType := Success

	msgs, err := consumeResponse(worker.channel, worker.queueName)
	if err != nil {
		t.Fatalf("Failed to consume messages: %s", err)
	}

	err = createTask("../tests/mock_files/Task.zip")
	if err != nil {
		t.Fatalf("Failed to create task: %s", err)
	}

	err = submitSubmission(testType, "1", "1")
	if err != nil {
		t.Fatalf("Failed to submit submission: %s", err)
	}

	err = publishTask(worker.channel)
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	message := <-msgs
	expectedResponse := generateExpectedResponseMessage(testType)
	if string(message.Body) != expectedResponse {
		t.Fatalf("Expected response: %s, got: %s", expectedResponse, string(message.Body))
	}

	err = deleteTask("1")
	if err != nil {
		t.Fatalf("Failed to delete task: %s", err)
	}

	tearDown(worker)
}

func TestFailedTimeLimitExceeded(t *testing.T) {
	worker := setup()
	testType := FailedTimeLimitExceeded

	msgs, err := consumeResponse(worker.channel, worker.queueName)
	if err != nil {
		t.Fatalf("Failed to consume messages: %s", err)
	}

	err = createTask("../tests/mock_files/Task.zip")
	if err != nil {
		t.Fatalf("Failed to create task: %s", err)
	}

	err = submitSubmission(testType, "1", "1")
	if err != nil {
		t.Fatalf("Failed to submit submission: %s", err)
	}

	err = publishTask(worker.channel)
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	message := <-msgs
	expectedResponse := generateExpectedResponseMessage(testType)
	if string(message.Body) != expectedResponse {
		t.Fatalf("Expected response: %s, got: %s", expectedResponse, string(message.Body))
	}

	err = deleteTask("1")
	if err != nil {
		t.Fatalf("Failed to delete task: %s", err)
	}
	tearDown(worker)
}

func TestCompilationError(t *testing.T) {
	worker := setup()
	testType := CompilationError

	msgs, err := consumeResponse(worker.channel, worker.queueName)
	if err != nil {
		t.Fatalf("Failed to consume messages: %s", err)
	}

	err = createTask("../tests/mock_files/Task.zip")
	if err != nil {
		t.Fatalf("Failed to create task: %s", err)
	}

	err = submitSubmission(testType, "1", "1")
	if err != nil {
		t.Fatalf("Failed to submit submission: %s", err)
	}

	err = publishTask(worker.channel)
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	message := <-msgs
	expectedResponse := generateExpectedResponseMessage(testType)
	if string(message.Body) != expectedResponse {
		t.Fatalf("Expected response: %s, got: %s", expectedResponse, string(message.Body))
	}

	err = deleteTask("1")
	if err != nil {
		t.Fatalf("Failed to delete task: %s", err)
	}
	tearDown(worker)
}

func TestNotExistingTask(t *testing.T) {
	worker := setup()

	msgs, err := consumeResponse(worker.channel, worker.queueName)
	if err != nil {
		t.Fatalf("Failed to consume messages: %s", err)
	}

	err = publishTask(worker.channel)
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	message := <-msgs
	if !strings.Contains(string(message.Body), "input src directory does not exist") {
		t.Fatalf("Expected response to contain 'input src directory does not exist', got: %s", string(message.Body))
	}

	tearDown(worker)
}

func TestNotExistingSolution(t *testing.T) {
	worker := setup()

	msgs, err := consumeResponse(worker.channel, worker.queueName)
	if err != nil {
		t.Fatalf("Failed to consume messages: %s", err)
	}

	err = createTask("../tests/mock_files/Task.zip")
	if err != nil {
		t.Fatalf("Failed to create task: %s", err)
	}

	err = publishTask(worker.channel)
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	message := <-msgs
	if !strings.Contains(string(message.Body), "solution file does not exist") {
		t.Fatalf("Expected response to contain 'solution file does not exist', got: %s", string(message.Body))
	}

	err = deleteTask("1")
	if err != nil {
		t.Fatalf("Failed to delete task: %s", err)
	}
	tearDown(worker)
}

func TestMain(m *testing.M) {
	logger.SetLoggerLevel(zapcore.ErrorLevel)
	code := m.Run()

	if code != 0 {
		err := deleteTask("1")
		if err != nil {
			fmt.Printf("Failed to delete task: %s", err)
		}
	}
	os.Exit(code)
}
