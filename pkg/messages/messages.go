package messages

import (
	"encoding/json"

	"github.com/mini-maxit/worker/pkg/constants"
)

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type ResponseQueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Ok        bool            `json:"ok"`
	Payload   json.RawMessage `json:"payload"`
}

type WorkerStatus struct {
	WorkerID            int                    `json:"worker_id"`
	Status              constants.WorkerStatus `json:"status"`
	ProcessingMessageID string                 `json:"processing_message_id"`
}

type ResponseWorkerStatusPayload struct {
	BusyWorkers  int            `json:"busy_workers"`
	TotalWorkers int            `json:"total_workers"`
	WorkerStatus []WorkerStatus `json:"worker_status"`
}

type LanguageSpec struct {
	LanguageName string   `json:"name"`
	Versions     []string `json:"versions"`
	Extension    string   `json:"extension"`
}

type ResponseHandshakePayload struct {
	Languages []LanguageSpec `json:"languages"`
}

type ResponseErrorPayload struct {
	Error string `json:"error"`
}

type FileLocation struct {
	ServerType string `json:"server_type"` // e.g., "s3", "local". FOR NOW INGORE
	Bucket     string `json:"bucket"`      // e.g., "submissions", "tasks", "results"
	Path       string `json:"path"`        // e.g., "42/main.cpp"
}

type TestCase struct {
	Order          int          `json:"order"`
	InputFile      FileLocation `json:"input_file"`
	ExpectedOutput FileLocation `json:"expected_output"`
	StdOutResult   FileLocation `json:"stdout_result"`
	StdErrResult   FileLocation `json:"stderr_result"`
	DiffResult     FileLocation `json:"diff_result"`
	TimeLimitMs    int64        `json:"time_limit_ms"`
	MemoryLimitKB  int64        `json:"memory_limit_kb"`
}

type TaskQueueMessage struct {
	LanguageType     string       `json:"language_type"`
	LanguageVersion  string       `json:"language_version"`
	SubmissionFile   FileLocation `json:"submission_file"`
	TestCases        []TestCase   `json:"test_cases"`
	TaskFilesVersion string       `json:"task_files_version"` // Version/timestamp to track task file updates
}
