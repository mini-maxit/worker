//go:build integration

package docker_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/mini-maxit/worker/internal/docker"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
)

const (
	testVolumePrefix  = "docker_client_test_vol_"
	testContainerName = "docker_client_test_container_"
)

var (
	// testLanguages contains all supported languages to test.
	testLanguages []languages.LanguageType
	// testImageName is set from the first supported language.
	testImageName string
)

func setupTestLanguages() {
	// Get all supported languages
	supportedLangs := languages.GetSupportedLanguages()
	for _, langStr := range supportedLangs {
		if lang, err := languages.ParseLanguageType(langStr); err == nil {
			testLanguages = append(testLanguages, lang)
		}
	}

	// Use the first language's image for basic tests
	if len(testLanguages) > 0 {
		if image, err := testLanguages[0].GetDockerImage(""); err == nil {
			testImageName = image
		}
	}
}

// TestNewDockerClient_Success tests successful creation of a docker client.
func TestNewDockerClient_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test languages on first test run
	if len(testLanguages) == 0 {
		setupTestLanguages()
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	if dc == nil {
		t.Fatal("docker client is nil")
	}
}

// TestNewDockerClient_WithVolume tests creating a client with a volume check.
func TestNewDockerClient_WithVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	volumeName := createTestVolume(t)
	defer cleanupVolume(t, volumeName)

	containerID := createContainerWithVolume(t, volumeName)
	defer cleanupContainer(t, containerID)

	dc, err := docker.NewDockerClient(volumeName)
	if err != nil {
		t.Fatalf("failed to create docker client with volume: %v", err)
	}
	if dc.DataVolumeName() != volumeName {
		t.Errorf("expected volume name %s, got %s", volumeName, dc.DataVolumeName())
	}
}

// TestDataVolumeName tests the DataVolumeName getter.
func TestDataVolumeName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	if dc.DataVolumeName() != "" {
		t.Errorf("expected empty volume name, got %s", dc.DataVolumeName())
	}
}

// TestCheckDataVolume_NotMounted tests error when volume is not mounted.
func TestCheckDataVolume_NotMounted(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	err = dc.CheckDataVolume("non_existent_volume_test")
	if !errors.Is(err, pkgerrors.ErrVolumeNotMounted) {
		t.Errorf("expected ErrVolumeNotMounted, got %v", err)
	}
}

// TestCheckDataVolume_Mounted tests successful volume mount check.
func TestCheckDataVolume_Mounted(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	volumeName := createTestVolume(t)
	defer cleanupVolume(t, volumeName)

	containerID := createContainerWithVolume(t, volumeName)
	defer cleanupContainer(t, containerID)

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	err = dc.CheckDataVolume(volumeName)
	if err != nil {
		t.Errorf("expected no error when checking mounted volume, got %v", err)
	}
}

// TestEnsureImage_PullImage tests pulling an image if it doesn't exist.
func TestEnsureImage_PullImage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	removeImageIfExists(t, testImageName)

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	verifyImageExists(t, testImageName)
}

// TestEnsureImage_AllSupportedLanguages tests pulling all supported language runtime images.
func TestEnsureImage_AllSupportedLanguages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if len(testLanguages) == 0 {
		t.Skip("No supported languages configured")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, lang := range testLanguages {
		langName := lang.String()
		imageName, err := lang.GetDockerImage("")
		if err != nil {
			t.Fatalf("failed to get docker image for language %s: %v", langName, err)
		}

		t.Logf("Testing language: %s (image: %s)", langName, imageName)

		// Ensure the image can be pulled
		err = dc.EnsureImage(ctx, imageName)
		if err != nil {
			t.Errorf("failed to ensure image for language %s: %v", langName, err)
			continue
		}

		// Verify the image exists
		verifyImageExists(t, imageName)
	}
}

// TestEnsureImage_ImageAlreadyExists tests when image is already present.
func TestEnsureImage_ImageAlreadyExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image first time: %v", err)
	}

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Errorf("expected no error when image already exists, got %v", err)
	}
}

// TestCreateAndStartContainer_Success tests creating and starting a container.
func TestCreateAndStartContainer_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	containerCfg := &container.Config{
		Image:      testImageName,
		Cmd:        []string{"echo", "hello world"},
		Entrypoint: nil,
	}

	hostCfg := &container.HostConfig{}
	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container: %v", err)
	}
	defer cleanupContainer(t, containerID)

	if containerID == "" {
		t.Fatal("container ID is empty")
	}

	verifyContainerExists(t, containerID)
}

// TestWaitContainer_Success tests waiting for a container to finish.
func TestWaitContainer_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sh", "-c", "echo hello"},
	}

	hostCfg := &container.HostConfig{}
	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container: %v", err)
	}
	defer cleanupContainer(t, containerID)

	exitCode, err := dc.WaitContainer(ctx, containerID)
	if err != nil {
		t.Errorf("failed to wait for container: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

// TestWaitContainer_WithTimeout tests waiting for a container with timeout.
func TestWaitContainer_WithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sh", "-c", "sleep 2"},
	}

	hostCfg := &container.HostConfig{}
	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container: %v", err)
	}
	defer cleanupContainer(t, containerID)

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()

	_, err = dc.WaitContainer(shortCtx, containerID)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// TestContainerWait_Success tests the ContainerWait method.
func TestContainerWait_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sh", "-c", "exit 5"},
	}

	hostCfg := &container.HostConfig{}
	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container: %v", err)
	}
	defer cleanupContainer(t, containerID)

	statusCh, errCh := dc.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		t.Errorf("unexpected error from ContainerWait: %v", err)
	case status := <-statusCh:
		if status.StatusCode != 5 {
			t.Errorf("expected exit code 5, got %d", status.StatusCode)
		}
	case <-time.After(10 * time.Second):
		t.Error("ContainerWait timed out")
	}
}

// TestContainerKill tests killing a running container.
func TestContainerKill(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sh", "-c", "sleep 100"},
	}

	hostCfg := &container.HostConfig{}
	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container: %v", err)
	}
	defer cleanupContainer(t, containerID)

	err = dc.ContainerKill(ctx, containerID, "KILL")
	if err != nil {
		t.Errorf("failed to kill container: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	verifyContainerStopped(t, containerID)
}

// TestCreateContainerWithVolume tests creating a container with a mounted volume.
func TestCreateContainerWithVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dc, err := docker.NewDockerClient("")
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = dc.EnsureImage(ctx, testImageName)
	if err != nil {
		t.Fatalf("failed to ensure image: %v", err)
	}

	volumeName := createTestVolume(t)
	defer cleanupVolume(t, volumeName)

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sh", "-c", "echo hello > " + constants.TmpDirPath + "/test.txt && sleep 1"},
	}

	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: constants.TmpDirPath,
			},
		},
	}

	containerName := fmt.Sprintf("%s%d", testContainerName, time.Now().UnixNano())

	containerID, err := dc.CreateAndStartContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		t.Fatalf("failed to create and start container with volume: %v", err)
	}
	defer cleanupContainer(t, containerID)

	exitCode, err := dc.WaitContainer(ctx, containerID)
	if err != nil {
		t.Errorf("failed to wait for container: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

// Helper functions

func createTestVolume(t *testing.T) string {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to create docker client for volume creation: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	volumeName := fmt.Sprintf("%s%d", testVolumePrefix, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = cli.VolumeCreate(ctx, volume.CreateOptions{Name: volumeName})
	if err != nil {
		t.Fatalf("failed to create test volume: %v", err)
	}

	return volumeName
}

func cleanupVolume(t *testing.T, volumeName string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for volume cleanup: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := cli.VolumeRemove(ctx, volumeName, false); err != nil {
		t.Logf("failed to remove test volume %s: %v", volumeName, err)
	}
}

func createContainerWithVolume(t *testing.T, volumeName string) string {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to create docker client for container creation: %v", err)
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = cli.ImageInspect(ctx, testImageName)
	if err != nil {
		reader, err := cli.ImagePull(ctx, testImageName, image.PullOptions{})
		if err != nil {
			t.Fatalf("failed to pull image: %v", err)
		}
		defer reader.Close()
		_, _ = io.Copy(io.Discard, reader)
	}

	containerCfg := &container.Config{
		Image: testImageName,
		Cmd:   []string{"sleep", "1000"},
	}

	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: constants.TmpDirPath,
			},
		},
	}

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, "")
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	return resp.ID
}

func cleanupContainer(t *testing.T, containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for container cleanup: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = cli.ContainerKill(ctx, containerID, "KILL")

	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		t.Logf("failed to remove test container %s: %v", containerID, err)
	}
}

func removeImageIfExists(t *testing.T, imageName string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for image removal: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		t.Logf("failed to list images: %v", err)
		return
	}

	for _, img := range images {
		for _, repoTag := range img.RepoTags {
			if repoTag == imageName {
				_, err := cli.ImageRemove(ctx, imageName, image.RemoveOptions{})
				if err != nil {
					t.Logf("failed to remove image %s: %v", imageName, err)
				}
				return
			}
		}
	}
}

func verifyImageExists(t *testing.T, imageName string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for image verification: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = cli.ImageInspect(ctx, imageName)
	if err != nil {
		t.Errorf("failed to verify image exists: %v", err)
	}
}

func verifyContainerExists(t *testing.T, containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for container verification: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = cli.ContainerInspect(ctx, containerID)
	if err != nil {
		t.Errorf("failed to verify container exists: %v", err)
	}
}

func verifyContainerStopped(t *testing.T, containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Logf("failed to create docker client for container status verification: %v", err)
		return
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		t.Errorf("failed to inspect container: %v", err)
		return
	}

	if inspect.State.Running {
		t.Errorf("expected container to be stopped, but it's still running")
	}
}
