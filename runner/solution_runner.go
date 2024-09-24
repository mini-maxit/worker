package runner

type SolutionRunner interface {
	RunSolution() SolutionResult
}

type SolutionResult struct {
}
