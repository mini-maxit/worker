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
	"github.com/mini-maxit/worker/pkg/solution"

	// "github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type Packager interface {
	PrepareSolutionPackage(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*TaskDirConfig, error)
	SendSolutionPackage(dirConfig *TaskDirConfig, task *messages.TaskQueueMessage, statusCode solution.ResultStatus) error
}

type packager struct {
	logger  *zap.SugaredLogger
	storage storage.Storage
}

type TaskDirConfig struct {
	TmpDirPath            string
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

func (p *packager) PrepareSolutionPackage(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*TaskDirConfig, error) {
	if p.storage == nil {
		return nil, errors.New("storage service is not initialized")
	}

	basePath := filepath.Join("/tmp", msgID)

	// create directories
	p.logger.Infof("Creating base directory at %s", basePath)
	if err := p.createBaseDirs(basePath); err != nil {
		p.logger.Errorf("Failed to create base directory: %s", err)
		// _ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download submission
	p.logger.Infof("Downloading submission file to %s", basePath)
	if err := p.downloadSubmission(basePath, taskQueueMessage.SubmissionFile); err != nil {
		p.logger.Errorf("Failed to download submission file: %s", err)
		// _ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	// Download test cases and create user files
	p.logger.Infof("Preparing test case files in %s", basePath)
	for idx, tc := range taskQueueMessage.TestCases {
		if err := p.prepareTestCaseFiles(basePath, idx, tc); err != nil {
			p.logger.Errorf("Failed to prepare test case files: %s", err)
			// _ = utils.RemoveIO(basePath, true, true)
			return nil, err
		}
	}

	// Create compile.err file
	p.logger.Infof("Creating compile.err file in %s", basePath)
	err := p.createCompileErrFile(basePath)
	if err != nil {
		p.logger.Errorf("Failed to create compile.err file: %s", err)
		// _ = utils.RemoveIO(basePath, true, true)
		return nil, err
	}

	cfg := &TaskDirConfig{
		TmpDirPath:            basePath,
		InputDirPath:          filepath.Join(basePath, "inputs"),
		OutputDirPath:         filepath.Join(basePath, "outputs"),
		UserSolutionPath:      filepath.Join(basePath, filepath.Base(taskQueueMessage.SubmissionFile.Path)),
		UserExecFilePath:      filepath.Join(basePath, strings.TrimSuffix(filepath.Base(taskQueueMessage.SubmissionFile.Path), filepath.Ext(taskQueueMessage.SubmissionFile.Path))),
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

func (p *packager) SendSolutionPackage(
	dirConfig *TaskDirConfig,
	task *messages.TaskQueueMessage,
	statusCode solution.ResultStatus,

) error {
	err := p.uploadNonEmptyFiles(dirConfig.UserOutputDirPath, task.UserOutputBase, ".out")
	if err != nil {
		return err
	}

	err = p.uploadNonEmptyFiles(dirConfig.UserErrorDirPath, task.UserErrorBase, ".err")
	if err != nil {
		return err
	}

	err = p.uploadNonEmptyFiles(dirConfig.UserDiffDirPath, task.UserDiffBase, ".diff")
	if err != nil {
		return err
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

// func (fs *fileService) moveCompilationError(taskDir, outputDir string) error {
// 	src := filepath.Join(taskDir, constants.CompileErrorFileName)
// 	dst := filepath.Join(outputDir, constants.CompileErrorFileName)
// 	return os.Rename(src, dst)
// }

// func (fs *fileService) sendArchiveToStorage(
// 	archiveFilePath string,
// 	userID, taskID, submissionNumber int64,
// ) error {
// 	body, writer, err := fs.prepareMultipartBody(archiveFilePath, userID, taskID, submissionNumber)
// 	if err != nil {
// 		return err
// 	}

// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	req, err := fs.createRequestWithContext(ctx, body, writer.FormDataContentType())
// 	if err != nil {
// 		return err
// 	}

// 	return fs.sendRequestAndHandleResponse(req)
// }

// func (fs *fileService) prepareMultipartBody(
// 	archiveFilePath string,
// 	userID, taskID, submissionNumber int64,
// ) (*bytes.Buffer, *multipart.Writer, error) {
// 	file, err := os.Open(archiveFilePath)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	defer file.Close()

// 	body := &bytes.Buffer{}
// 	writer := multipart.NewWriter(body)

// 	form := map[string]string{
// 		"userID":           strconv.FormatInt(userID, 10),
// 		"taskID":           strconv.FormatInt(taskID, 10),
// 		"submissionNumber": strconv.FormatInt(submissionNumber, 10),
// 	}

// 	for key, val := range form {
// 		if err := writer.WriteField(key, val); err != nil {
// 			return nil, nil, err
// 		}
// 	}

// 	part, err := writer.CreateFormFile("archive", filepath.Base(archiveFilePath))
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	if _, err := io.Copy(part, file); err != nil {
// 		return nil, nil, err
// 	}

// 	if err := writer.Close(); err != nil {
// 		return nil, nil, err
// 	}

// 	return body, writer, nil
// }

// func (fs *fileService) createRequestWithContext(
// 	ctx context.Context,
// 	body *bytes.Buffer,
// 	contentType string,
// ) (*http.Request, error) {
// 	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fs.fileStorageURL+"/storeOutputs", body)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header.Set("Content-Type", contentType)
// 	return req, nil
// }

// func (fs *fileService) sendRequestAndHandleResponse(req *http.Request) error {
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		buf := new(bytes.Buffer)
// 		if _, err := buf.ReadFrom(resp.Body); err != nil {
// 			fs.logger.Errorf("Failed to read response body: %s", err)
// 		}
// 		fs.logger.Errorf("Failed to store solution result: %s", buf.String())
// 		return cutomErrors.ErrFailedToStoreSolution
// 	}

// 	return nil
// }
