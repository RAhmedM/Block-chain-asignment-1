// pkg/rpc/rpc.go

package rpc

import (
	"matrix-compute/pkg/matrix"
)

// Operation represents the type of matrix operation
type Operation int

const (
	Addition Operation = iota
	Transpose
	Multiplication
)

// Task represents a matrix computation task
type Task struct {
	ID        string
	Op        Operation
	Matrix1   *matrix.Matrix
	Matrix2   *matrix.Matrix // Used for Addition and Multiplication
	Result    *matrix.Matrix
	Completed bool
	Error     string
}

// ComputeRequest represents a client's computation request
type ComputeRequest struct {
	TaskID    string
	Op        Operation
	Matrix1   *matrix.Matrix
	Matrix2   *matrix.Matrix // Optional, depending on operation
}

// ComputeResponse represents the server's response to a computation request
type ComputeResponse struct {
	TaskID  string
	Result  *matrix.Matrix
	Error   string
	Success bool
}

// WorkerStatus represents the current status of a worker
type WorkerStatus struct {
	ID          string
	Available   bool
	TaskCount   int
	LastUpdated int64 // Unix timestamp
}

// CoordinatorService defines the RPC methods that will be exposed
type CoordinatorService interface {
	SubmitTask(req *ComputeRequest, resp *ComputeResponse) error
	GetTaskStatus(taskID string, status *ComputeResponse) error
}

// WorkerService defines the RPC methods that workers will expose
type WorkerService interface {
	ExecuteTask(task *Task, result *ComputeResponse) error
	GetStatus(dummy *struct{}, status *WorkerStatus) error
}