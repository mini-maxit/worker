package solution

type ResultStatus int

const (
	// Means the solution executed successfully without any errors and all test cases passed.
	Success ResultStatus = iota + 1
	// Means that some test casses failed (difference, timeout, memory limit exceeded etc.)
	TestFailed
	// Means that the solution failed to compile.
	CompilationError
	// Means that the solution failed to initialize wrong language, version etc.
	InitializationError
	// Means that some internal error occurred.
	InternalError
)

type TestCaseStatus int

const (
	// Means that the test case passed.
	TestCasePassed TestCaseStatus = iota + 1
	// Means the output differs from the expected output.
	OutputDifference
	// Means that the solution timed out.
	TimeLimitExceeded
	// Means that the solution exceeded the memory limit.
	MemoryLimitExceeded
	// Means that some runtime error occurred.
	RuntimeError
)

type Result struct {
	StatusCode  ResultStatus `json:"status_code"`  // Status of the solution execution
	Message     string       `json:"message"`      // any information message in case of error is error message
	TestResults []TestResult `json:"test_results"` // test results in case of error or success
}

type TestResult struct {
	Passed        bool           `json:"passed"` // Whether the test passed or failed
	ExecutionTime float64        `json:"execution_time"`
	PeakMem       int64          `json:"peak_memory"`
	StatusCode    TestCaseStatus `json:"status_code"` // Status of the test case
	// Error message in case of failure. Does not include information about difference in expected and actual output
	ErrorMessage string `json:"error_message"`
	Order        int    `json:"order"` // Order to input output pair for ex 1 mean in1.in and out1.out was used
}

type Limit struct {
	TimeMs   int64 // Time limit in milliseconds
	MemoryKb int64 // Memory limit in kilobytes
}
