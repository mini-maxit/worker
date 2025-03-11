package solution

type SolutionStatus int

const (
	// Means the solution executed successfully without any errors. Does not mean that all tests passed
	Success SolutionStatus = iota + 1
	// Means there was a compilation error while compiling the solution
	CompilationError
	// Means the solution failed to execute
	Failed
	// Means there was an internal error while executing the solution. This implies that the solution failed to execute
	InternalError
	// Means there was an error while initializing the executor
	InitializationError
	// Means there was an runtime error while executing the solution
	RuntimeError
)

func (ss SolutionStatus) String() string {
	switch ss {
	case Success:
		return "Success"
	case Failed:
		return "Failed"
	case InternalError:
		return "InternalError"
	case CompilationError:
		return "CompilationError"
	case InitializationError:
		return "InitializationError"
	case RuntimeError:
		return "RuntimeError"
	default:
		return "Unknown"
	}
}

type SolutionResult struct {
	OutputDir   string         // Directory where output files are stored
	Success     bool           // wether solution passed or failed
	StatusCode  SolutionStatus // Status of the solution execution
	Code        string         // any code in case of error or success depending on status code
	Message     string         // any information message in case of error is error message
	TestResults []TestResult   // test results in case of error or success
}
type TestResult struct {
	Passed       bool   // Whether the test passed or failed
	ErrorMessage string // Error message in case of failure (if any)
	Order        int    // Order to input output pair for ex 1 mean in1.in and out1.out was used
}
