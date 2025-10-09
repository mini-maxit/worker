package messages

import "encoding/json"

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

type FileLocation struct {
	Bucket string `json:"bucket"` // e.g., "submissions", "tasks", "results"
	Path   string `json:"path"`   // e.g., "42/main.cpp"
}

type TestCase struct {
	InputFile      FileLocation `json:"input_file"`
	ExpectedOutput FileLocation `json:"expected_output"`
	TimeLimitMs    int64        `json:"time_limit_ms"`
	MemoryLimitKB  int64        `json:"memory_limit_kb"`
}

type TaskQueueMessage struct {
	LanguageType    string       `json:"language_type"`
	LanguageVersion string       `json:"language_version"`
	SubmissionFile  FileLocation `json:"submission_file"`
	UserOutputBase  FileLocation `json:"user_output_base"`
	UserErrorBase   FileLocation `json:"user_error_base"`
	UserDiffBase    FileLocation `json:"user_diff_base"`
	TestCases       []TestCase   `json:"test_cases"`
}
