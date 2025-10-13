package packager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/storage"
	"github.com/mini-maxit/worker/pkg/constants"
	customErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"

	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type Packager interface {
	PrepareSolutionPackage(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*TaskDirConfig, error)
	SendSolutionPackage(dirConfig *TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error
}

type packager struct {
	logger  *zap.SugaredLogger
	storage storage.Storage
}

type TaskDirConfig struct {
	TmpDirPath            string
	PackageDirPath        string
	InputDirPath          string
	OutputDirPath         string
	UserSolutionPath      string
	UserExecFilePath      string
	UserOutputDirPath     string
	UserExecResultDirPath string
	UserErrorDirPath      string
	UserDiffDirPath       string
	CompileErrFilePath    string
}

func NewPackager(storage storage.Storage) Packager {
	logger := logger.NewNamedLogger("packager")
	return &packager{
		logger:  logger,
		storage: storage,
	}
}

func (p *packager) PrepareSolutionPackage(
	taskQueueMessage *messages.TaskQueueMessage,
	msgID string,
) (*TaskDirConfig, error) {
	if p.storage == nil {
		return nil, errors.New("storage service is not initialized")
	}

	basePath := filepath.Join(constants.TmpDirPath, msgID)

	// create directories.
	p.logger.Infof("Creating base directory at %s", basePath)
	if err := p.createBaseDirs(basePath); err != nil {
		p.logger.Errorf("Failed to create base directory: %s", err)
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download submission.
	p.logger.Infof("Downloading submission file to %s", basePath)
	if err := p.downloadSubmission(basePath, taskQueueMessage.SubmissionFile); err != nil {
		p.logger.Errorf("Failed to download submission file: %s", err)
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download test cases and create user files.
	p.logger.Infof("Preparing test case files in %s", basePath)
	for idx, tc := range taskQueueMessage.TestCases {
		if err := p.prepareTestCaseFiles(basePath, idx, tc); err != nil {
			p.logger.Errorf("Failed to prepare test case files: %s", err)
			_ = utils.RemoveIO(basePath, true, true)
			return nil, err
		}
	}

	// Create compile.err file.
	p.logger.Infof("Creating compile.err file in %s", basePath)
	err := p.createCompileErrFile(basePath)
	if err != nil {
		p.logger.Errorf("Failed to create compile.err file: %s", err)
		_ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	userFileExt := filepath.Ext(taskQueueMessage.SubmissionFile.Path)
	userFileNameWithoutExt := strings.TrimSuffix(filepath.Base(taskQueueMessage.SubmissionFile.Path), userFileExt)
	userExecPath := filepath.Join(basePath, userFileNameWithoutExt)

	cfg := &TaskDirConfig{
		TmpDirPath:            constants.TmpDirPath,
		PackageDirPath:        basePath,
		InputDirPath:          filepath.Join(basePath, constants.InputDirName),
		OutputDirPath:         filepath.Join(basePath, constants.OutputDirName),
		UserSolutionPath:      filepath.Join(basePath, filepath.Base(taskQueueMessage.SubmissionFile.Path)),
		UserExecFilePath:      userExecPath,
		UserOutputDirPath:     filepath.Join(basePath, constants.UserOutputDirName),
		UserErrorDirPath:      filepath.Join(basePath, constants.UserErrorDirName),
		UserDiffDirPath:       filepath.Join(basePath, constants.UserDiffDirName),
		CompileErrFilePath:    filepath.Join(basePath, constants.CompileErrFileName),
		UserExecResultDirPath: filepath.Join(basePath, constants.UserExecResultDirName),
	}

	p.logger.Infof("Prepared solution package at %s", basePath)
	return cfg, nil
}

// createBaseDirs creates the base temporary directory and required subfolders.
func (p *packager) createBaseDirs(basePath string) error {
	dirs := []string{
		basePath,
		filepath.Join(basePath, constants.InputDirName),
		filepath.Join(basePath, constants.OutputDirName),
		filepath.Join(basePath, constants.UserOutputDirName),
		filepath.Join(basePath, constants.UserErrorDirName),
		filepath.Join(basePath, constants.UserDiffDirName),
		filepath.Join(basePath, constants.UserExecResultDirName),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			p.logger.Errorf("Failed to create directory %s: %s", d, err)
			return err
		}
	}
	return nil
}

// downloadSubmission downloads the submission file into basePath/solution.
func (p *packager) downloadSubmission(basePath string, submission messages.FileLocation) error {
	if submission.Bucket == "" || submission.Path == "" {
		return customErr.ErrSubmissionFileLocationEmpty
	}
	path := filepath.Join(basePath, filepath.Base(submission.Path))
	if _, err := p.storage.DownloadFile(submission, path); err != nil {
		p.logger.Errorf("Failed to download submission file: %s", err)
		return err
	}
	return nil
}

// prepareTestCaseFiles downloads input and expected output for a test case and creates user files.
func (p *packager) prepareTestCaseFiles(basePath string, idx int, tc messages.TestCase) error {
	// inputs
	if tc.InputFile.Bucket == "" || tc.InputFile.Path == "" {
		p.logger.Warnf("Test case %d input location is empty, skipping", idx)
	} else {
		inputDest := filepath.Join(basePath, constants.InputDirName, filepath.Base(tc.InputFile.Path))
		if _, err := p.storage.DownloadFile(tc.InputFile, inputDest); err != nil {
			p.logger.Errorf("Failed to download input for test case %d: %s", idx, err)
			return err
		}
	}

	// expected outputs.
	if tc.ExpectedOutput.Bucket == "" || tc.ExpectedOutput.Path == "" {
		p.logger.Warnf("Test case %d expected output location is empty, skipping", idx)
	} else {
		outputDest := filepath.Join(basePath, constants.OutputDirName, filepath.Base(tc.ExpectedOutput.Path))
		if _, err := p.storage.DownloadFile(tc.ExpectedOutput, outputDest); err != nil {
			p.logger.Errorf("Failed to download expected output for test case %d: %s", idx, err)
			return err
		}
	}

	prefix := strings.TrimSuffix(filepath.Base(tc.InputFile.Path), filepath.Ext(tc.InputFile.Path))

	userOut := filepath.Join(basePath, constants.UserOutputDirName, prefix+".out")
	userErr := filepath.Join(basePath, constants.UserErrorDirName, prefix+".err")
	userDiff := filepath.Join(basePath, constants.UserDiffDirName, prefix+".diff")
	userRes := filepath.Join(basePath, constants.UserExecResultDirName, prefix+".res")

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
	compErrFilePath := filepath.Join(basePath, constants.CompileErrFileName)
	compErrFile, err := os.OpenFile(compErrFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		p.logger.Errorf("Failed to create compile.err file: %s", err)
		return err
	}
	compErrFile.Close()
	return nil
}

func (p *packager) SendSolutionPackage(
	dirConfig *TaskDirConfig,
	testCases []messages.TestCase,
	hasCompilationErr bool,
) error {
	if hasCompilationErr {
		err := p.uploadNonEmptyFiles(dirConfig.CompileErrFilePath, testCases[0].StdErrResult, ".err")
		if err != nil {
			return err
		}
		return nil
	}

	for _, test := range testCases {
		err := p.uploadNonEmptyFiles(dirConfig.UserOutputDirPath, test.StdOutResult, ".out")
		if err != nil {
			return err
		}

		err = p.uploadNonEmptyFiles(dirConfig.UserErrorDirPath, test.StdErrResult, ".err")
		if err != nil {
			return err
		}

		err = p.uploadNonEmptyFiles(dirConfig.UserDiffDirPath, test.DiffResult, ".diff")
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *packager) uploadNonEmptyFiles(dirPath string, outputFileLocation messages.FileLocation, fileExt string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		p.logger.Errorf("Failed to read directory %s: %s", dirPath, err)
		return err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == fileExt {
			filePath := filepath.Join(dirPath, file.Name())
			if fi, err := os.Stat(filePath); err == nil {
				if fi.Size() == 0 {
					continue
				}
			}

			p.logger.Infof("Uploading file: %s", filePath)
			if err := p.storage.UploadFile(filePath, outputFileLocation.Bucket, outputFileLocation.Path); err != nil {
				p.logger.Errorf("Failed to upload file %s: %s", filePath, err)
				return err
			}
		}
	}

	return nil
}
