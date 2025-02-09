package solution

type Solution struct {
	Language         LanguageConfig // Language of the solution
	TimeLimits       []int          // Time limit for the solution
	MemoryLimits     []int          // Memory limit for the
	BaseDir          string         // Base directory where solution file is stored
	SolutionFileName string         // Name of the solution file
	InputDir         string         // Directory where input files are stored
	OutputDir        string         // Directory where expected output files are stored
}

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
