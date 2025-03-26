package solution

type SolutionStatus int

const (
	// Means the solution executed successfully without any errors. Does not mean that all tests passed
	Success SolutionStatus = iota + 1
	// Means that some test casses did not pass
	TestFailed
	// Means that the solution timed out
	TimeLimitExceeded
	// Means that the solution exceeded the memory limit
	MemoryLimitExceeded
	// Means that the solution failed to compile
	CompilationError
	// Means that the solution failed to initialize wrong language, version etc
	InitializationError
	// Means that some internal error occurred
	InternalError
	// Means that some runtime error occurred
	RuntimeError
)

type SolutionResult struct {
	OutputDir   string         // Directory where output files are stored
	StatusCode  SolutionStatus // Status of the solution execution
	Message     string         // any information message in case of error is error message
	TestResults []TestResult   // test results in case of error or success
}
type TestResult struct {
	Passed       bool   // Whether the test passed or failed
	ErrorMessage string // Error message in case of failure (if any). Does not include information about difference in expected and actual output
	Order        int    // Order to input output pair for ex 1 mean in1.in and out1.out was used
}
