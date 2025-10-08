package packager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/storage"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type Packager interface {
	PrepareSolutionPackage(taskQueueMessage messages.TaskQueueMessage, msgID string) (*TaskDirConfig, error)
}

type packager struct {
	logger  *zap.SugaredLogger
	storage storage.FileService
}

type TaskDirConfig struct {
	TmpDirPath            string
	InputDirPath          string
	OutputDirPath         string
	UserSolutionPath      string
	UserOutputDirPath     string
	UserExecResultDirPath string
	UserErrorDirPath      string
	UserDiffDirPath       string
	CompileErrFilePath    string
}

// Create user output directory
// Set up output, error, diff files
//

func NewPackager() Packager {
	logger := logger.NewNamedLogger("packager")
	return &packager{
		logger: logger,
	}
}

func (p *packager) PrepareSolutionPackage(taskQueueMessage messages.TaskQueueMessage, msgID string) (*TaskDirConfig, error) {
	if p.storage == nil {
		return nil, errors.New("storage service is not initialized")
	}

	basePath := filepath.Join("/tmp", msgID)

	// create directories
	if err := p.createBaseDirs(basePath); err != nil {
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download submission
	if err := p.downloadSubmission(basePath, taskQueueMessage.SubmissionFile); err != nil {
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download test cases and create user files
	for idx, tc := range taskQueueMessage.TestCases {
		if err := p.handleTestCase(basePath, idx, tc); err != nil {
			_ = utils.RemoveIO(basePath, true, true)
			return nil, err
		}
	}

	// Create compile.err file
	err := p.createCompileErrFile(basePath)
	if err != nil {
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	cfg := &TaskDirConfig{
		TmpDirPath:            basePath,
		InputDirPath:          filepath.Join(basePath, "inputs"),
		OutputDirPath:         filepath.Join(basePath, "outputs"),
		UserSolutionPath:      filepath.Join(basePath, "solution"),
		UserOutputDirPath:     filepath.Join(basePath, "userOutputs"),
		UserErrorDirPath:      filepath.Join(basePath, "userErrors"),
		UserDiffDirPath:       filepath.Join(basePath, "userDiff"),
		CompileErrFilePath:    filepath.Join(basePath, "compile.err"),
		UserExecResultDirPath: filepath.Join(basePath, "userExecResults"),
	}

	p.logger.Infof("Prepared solution package at %s", basePath)
	return cfg, nil
}

// createBaseDirs creates the base temporary directory and required subfolders.
func (p *packager) createBaseDirs(basePath string) error {
	dirs := []string{
		basePath,
		filepath.Join(basePath, "solution"),
		filepath.Join(basePath, "inputs"),
		filepath.Join(basePath, "outputs"),
		filepath.Join(basePath, "userOutputs"),
		filepath.Join(basePath, "userErrors"),
		filepath.Join(basePath, "userDiff"),
		filepath.Join(basePath, "userExecResults"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			p.logger.Errorf("Failed to create directory %s: %s", d, err)
			return err
		}
	}
	return nil
}

// downloadSubmission downloads the submission file into basePath/solution
func (p *packager) downloadSubmission(basePath string, submission messages.FileLocation) error {
	if submission.Bucket == "" || submission.Path == "" {
		return fmt.Errorf("submission file location is empty")
	}
	solutionDest := filepath.Join(basePath, "solution")
	if _, err := p.storage.DownloadFile(submission, solutionDest); err != nil {
		p.logger.Errorf("Failed to download submission file: %s", err)
		return err
	}
	return nil
}

// handleTestCase downloads input and expected output for a test case and creates user files.
func (p *packager) handleTestCase(basePath string, idx int, tc messages.TestCase) error {
	// inputs
	if tc.InputFile.Bucket == "" || tc.InputFile.Path == "" {
		p.logger.Warnf("Test case %d input location is empty, skipping", idx)
	} else {
		inputDest := filepath.Join(basePath, "inputs", filepath.Base(tc.InputFile.Path))
		if _, err := p.storage.DownloadFile(tc.InputFile, inputDest); err != nil {
			p.logger.Errorf("Failed to download input for test case %d: %s", idx, err)
			return err
		}
	}

	// expected outputs
	if tc.ExpectedOutput.Bucket == "" || tc.ExpectedOutput.Path == "" {
		p.logger.Warnf("Test case %d expected output location is empty, skipping", idx)
	} else {
		outputDest := filepath.Join(basePath, "outputs", filepath.Base(tc.ExpectedOutput.Path))
		if _, err := p.storage.DownloadFile(tc.ExpectedOutput, outputDest); err != nil {
			p.logger.Errorf("Failed to download expected output for test case %d: %s", idx, err)
			return err
		}
	}

	// Create user files: X.out, X.err, X.diff using input filename stem or index fallback
	var prefix string
	if tc.InputFile.Path != "" {
		prefix = strings.TrimSuffix(filepath.Base(tc.InputFile.Path), filepath.Ext(tc.InputFile.Path))
	} else {
		prefix = strconv.Itoa(idx + 1)
	}

	userOut := filepath.Join(basePath, "userOutputs", prefix+".out")
	userErr := filepath.Join(basePath, "userErrors", prefix+".err")
	userDiff := filepath.Join(basePath, "userDiff", prefix+".diff")
	userRes := filepath.Join(basePath, "userExecResults", prefix+".res")

	for _, f := range []string{userOut, userErr, userDiff, userRes} {
		fi, err := os.OpenFile(f, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			p.logger.Errorf("Failed to create user file %s: %s", f, err)
			return err
		}
		fi.Close()
	}
	return nil
}

func (p *packager) createCompileErrFile(basePath string) error {
	compErrFilePath := filepath.Join(basePath, "compile.err")
	compErrFile, err := os.OpenFile(compErrFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		p.logger.Errorf("Failed to create compile.err file: %s", err)
		return err
	}
	compErrFile.Close()
	return nil
}
