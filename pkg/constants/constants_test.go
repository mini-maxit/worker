package constants

import (
	"encoding/json"
	"testing"
)

// TestWorkerStatusString tests the String() method of WorkerStatus enum.
func TestWorkerStatusString(t *testing.T) {
	tests := []struct {
		name     string
		status   WorkerStatus
		expected string
	}{
		{
			name:     "idle status",
			status:   WorkerStatusIdle,
			expected: "idle",
		},
		{
			name:     "busy status",
			status:   WorkerStatusBusy,
			expected: "busy",
		},
		{
			name:     "unknown status",
			status:   WorkerStatus(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Errorf("WorkerStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestWorkerStatusMarshalJSON tests JSON serialization of WorkerStatus enum.
func TestWorkerStatusMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		status   WorkerStatus
		expected string
	}{
		{
			name:     "idle status marshals to string",
			status:   WorkerStatusIdle,
			expected: `"idle"`,
		},
		{
			name:     "busy status marshals to string",
			status:   WorkerStatusBusy,
			expected: `"busy"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(got) != tt.expected {
				t.Errorf("json.Marshal() = %v, want %v", string(got), tt.expected)
			}
		})
	}
}

// TestWorkerStatusInMap tests that WorkerStatus enum works correctly in maps
// as it would be used in GetWorkersStatus().
func TestWorkerStatusInMap(t *testing.T) {
	// Create a status map similar to what GetWorkersStatus returns
	statusMap := map[string]interface{}{
		"busy_workers":  1,
		"total_workers": 2,
		"worker_status": map[int]string{
			0: WorkerStatusIdle.String(),
			1: WorkerStatusBusy.String() + " Processing message: msg-123",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(statusMap)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal back to verify structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify the structure
	if result["busy_workers"].(float64) != 1 {
		t.Errorf("busy_workers = %v, want 1", result["busy_workers"])
	}
	if result["total_workers"].(float64) != 2 {
		t.Errorf("total_workers = %v, want 2", result["total_workers"])
	}

	workerStatus := result["worker_status"].(map[string]interface{})
	if workerStatus["0"] != "idle" {
		t.Errorf("worker 0 status = %v, want 'idle'", workerStatus["0"])
	}
	if workerStatus["1"] != "busy Processing message: msg-123" {
		t.Errorf("worker 1 status = %v, want 'busy Processing message: msg-123'", workerStatus["1"])
	}
}
