package config_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/pkg/constants"
)

func TestRabbitmqConfig_DefaultsAndCustom(t *testing.T) {
	config := NewConfig()
	expectedURL := fmt.Sprintf(
		"amqp://%s:%s@%s:%s/",
		constants.DefaultRabbitmqUser,
		constants.DefaultRabbitmqPassword,
		constants.DefaultRabbitmqHost,
		constants.DefaultRabbitmqPort)

	if config.RabbitMQURL != expectedURL {
		t.Fatalf("expected url %q, got %q", expectedURL, config.RabbitMQURL)
	}
	if config.PublishChanSize != constants.DefaultRabbitmqPublishChanSize {
		t.Fatalf("expected publish chan size %d, got %d", constants.DefaultRabbitmqPublishChanSize, config.PublishChanSize)
	}

	// custom values
	t.Setenv("RABBITMQ_HOST", "rm-host")
	t.Setenv("RABBITMQ_PORT", "12345")
	t.Setenv("RABBITMQ_USER", "u1")
	t.Setenv("RABBITMQ_PASSWORD", "p1")
	t.Setenv("RABBITMQ_PUBLISH_CHAN_SIZE", "7")

	config2 := NewConfig()
	expectedURL2 := fmt.Sprintf("amqp://%s:%s@%s:%s/", "u1", "p1", "rm-host", "12345")
	if config2.RabbitMQURL != expectedURL2 {
		t.Fatalf("expected url %q, got %q", expectedURL2, config2.RabbitMQURL)
	}
	if config2.PublishChanSize != 7 {
		t.Fatalf("expected publish chan size %d, got %d", 7, config2.PublishChanSize)
	}
}

func TestStorageConfig_DefaultsAndCustom(t *testing.T) {
	url := NewConfig().StorageBaseUrl
	expected := fmt.Sprintf("http://%s:%s", constants.DefaultStorageHost, constants.DefaultStoragePort)
	if url != expected {
		t.Fatalf("expected storage url %q, got %q", expected, url)
	}

	t.Setenv("STORAGE_HOST", "store-host")
	t.Setenv("STORAGE_PORT", "4321")
	url2 := NewConfig().StorageBaseUrl
	expected2 := fmt.Sprintf("http://%s:%s", "store-host", "4321")
	if url2 != expected2 {
		t.Fatalf("expected storage url %q, got %q", expected2, url2)
	}
}

func TestWorkerConfig_DefaultsAndCustom(t *testing.T) {
	config := NewConfig()
	if config.ConsumeQueueName != constants.DefaultWorkerQueueName {
		t.Fatalf("expected default worker queue name %q, got %q", constants.DefaultWorkerQueueName, config.ConsumeQueueName)
	}
	if config.MaxWorkers != constants.DefaultMaxWorkers {
		t.Fatalf("expected default max workers %d, got %d", constants.DefaultMaxWorkers, config.MaxWorkers)
	}

	t.Setenv("WORKER_QUEUE_NAME", "custom_queue")
	t.Setenv("MAX_WORKERS", "3")
	config2 := NewConfig()
	if config2.ConsumeQueueName != "custom_queue" {
		t.Fatalf("expected worker queue name %q, got %q", "custom_queue", config2.ConsumeQueueName)
	}
	if config2.MaxWorkers != 3 {
		t.Fatalf("expected max workers %d, got %d", 3, config2.MaxWorkers)
	}
}

func TestDockerConfig_DefaultsAndCustom(t *testing.T) {
	v := NewConfig().JobsDataVolume
	if v != constants.DefaultJobsDataVolume {
		t.Fatalf("expected default jobs data volume %q, got %q", constants.DefaultJobsDataVolume, v)
	}

	t.Setenv("JOBS_DATA_VOLUME", "my-vol")
	v2 := NewConfig().JobsDataVolume
	if v2 != "my-vol" {
		t.Fatalf("expected jobs data volume %q, got %q", "my-vol", v2)
	}
}

func TestVerifierConfig_DefaultsAndCustom(t *testing.T) {
	flags := NewConfig().VerifierFlags
	if len(flags) != 1 || flags[0] != constants.DefaultVerifierFlags {
		t.Fatalf("expected default verifier flags [%s], got %v", constants.DefaultVerifierFlags, flags)
	}

	t.Setenv("VERIFIER_FLAGS", "-w,-B")
	flags2 := NewConfig().VerifierFlags
	if len(flags2) != 2 || flags2[0] != "-w" || flags2[1] != "-B" {
		t.Fatalf("expected verifier flags [-w -B], got %v", flags2)
	}
}

func TestNewConfig_PicksUpValues(t *testing.T) {
	// set a variety of envs and ensure NewConfig reads them
	t.Setenv("RABBITMQ_HOST", "xhost")
	t.Setenv("RABBITMQ_PORT", "1111")
	t.Setenv("RABBITMQ_USER", "u2")
	t.Setenv("RABBITMQ_PASSWORD", "p2")
	t.Setenv("RABBITMQ_PUBLISH_CHAN_SIZE", "4")
	t.Setenv("STORAGE_HOST", "s-host")
	t.Setenv("STORAGE_PORT", "2222")
	t.Setenv("WORKER_QUEUE_NAME", "q-name")
	t.Setenv("MAX_WORKERS", "5")
	t.Setenv("JOBS_DATA_VOLUME", "vol-1")
	t.Setenv("VERIFIER_FLAGS", "-a,-b")

	cfg := NewConfig()
	if cfg.RabbitMQURL == "" {
		t.Fatalf("expected non-empty RabbitMQURL")
	}
	// verify a couple of fields are propagated correctly
	if cfg.StorageBaseUrl != fmt.Sprintf("http://%s:%s", "s-host", "2222") {
		t.Fatalf("unexpected StorageBaseUrl: %s", cfg.StorageBaseUrl)
	}
	if cfg.ConsumeQueueName != "q-name" {
		t.Fatalf("unexpected ConsumeQueueName: %s", cfg.ConsumeQueueName)
	}
	if cfg.MaxWorkers != 5 {
		t.Fatalf("unexpected MaxWorkers: %d", cfg.MaxWorkers)
	}
	if len(cfg.VerifierFlags) != 2 || cfg.VerifierFlags[0] != "-a" || cfg.VerifierFlags[1] != "-b" {
		t.Fatalf("unexpected VerifierFlags: %v", cfg.VerifierFlags)
	}
	// ensure publish chan size parsed
	if cfg.PublishChanSize != 4 {
		t.Fatalf("unexpected PublishChanSize: %d", cfg.PublishChanSize)
	}
	// sanity check RabbitMQ URL contains provided host and port
	if !strings.Contains(cfg.RabbitMQURL, "xhost") || !strings.Contains(cfg.RabbitMQURL, "1111") {
		t.Fatalf("RabbitMQURL does not contain provided host/port: %s", cfg.RabbitMQURL)
	}
}
