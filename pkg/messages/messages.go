package messages

import "encoding/json"

type QueueMessage struct {
	Type      string          `json:"type"`
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

// TODO: Refactore based on the new Task structure
// type TaskQueueMessage struct {
// 	TaskID           int64  `json:"task_id"`
// 	UserID           int64  `json:"user_id"`
// 	SubmissionNumber int64  `json:"submission_number"`
// 	LanguageType     string `json:"language_type"`
// 	LanguageVersion  string `json:"language_version"`
// 	TimeLimits       []int  `json:"time_limits"`
// 	MemoryLimits     []int  `json:"memory_limits"`
// 	ChrootDirPath    string `json:"chroot_dir_path,omitempty"` // Optional for test purposes
// 	UseChroot        string `json:"use_chroot,omitempty"`      // Optional for test purposes
// }

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
	InputFile        FileLocation `json:"input_file"`
	ExpectedOutput   FileLocation `json:"expected_output"`
	StdoutResult     FileLocation `json:"stdout_result"`
	StderrResult     FileLocation `json:"stderr_result"`
	TimeLimitMs      int64        `json:"time_limit_ms"`
	MemoryLimitKB    int64        `json:"memory_limit_kb"`
}

type TaskQueueMessage struct {
	SubmissionNumber   int64        `json:"submission_number"`
	LanguageType       string       `json:"language_type"`
	LanguageVersion    string       `json:"language_version"`
	SubmissionFile     FileLocation `json:"submission_file"`
	TestCases          []TestCase   `json:"test_cases"`
}