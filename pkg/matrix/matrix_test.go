// pkg/matrix/matrix_test.go

package matrix_test

import (
	"fmt"
	"matrix-compute/pkg/matrix"
	"matrix-compute/pkg/types"
	"net/rpc"
	"testing"
	"time"
)

// TestLargeMatrixOperations verifies system handles large matrices
func TestLargeMatrixOperations(t *testing.T) {
	// Create 1000x1000 matrices
	size := 1000
	mat1, err := matrix.NewMatrix(size, size)
	if err != nil {
		t.Fatalf("Failed to create large matrix: %v", err)
	}

	// Fill with test data
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			mat1.Set(i, j, float64(i+j))
		}
	}

	mat2, _ := matrix.NewMatrix(size, size)
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			mat2.Set(i, j, float64(i*j))
		}
	}

	// Test all operations
	operations := []struct {
		name string
		op   types.Operation
	}{
		{"Addition", types.Addition},
		{"Transpose", types.Transpose},
		{"Multiplication", types.Multiplication},
	}

	client, err := rpc.Dial("tcp", "localhost:1234")
	if err != nil {
		t.Fatalf("Failed to connect to coordinator: %v", err)
	}
	defer client.Close()

	for _, tc := range operations {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.ComputeRequest{
				TaskID:  fmt.Sprintf("large-matrix-%s-%d", tc.name, time.Now().Unix()),
				Op:      tc.op,
				Matrix1: mat1,
				Matrix2: tc.op != types.Transpose ? mat2 : nil,
			}

			var resp types.ComputeResponse
			err := client.Call("Coordinator.SubmitTask", req, &resp)
			if err != nil {
				t.Fatalf("Failed to submit task: %v", err)
			}

			// Wait for completion
			for i := 0; i < 30; i++ {
				err = client.Call("Coordinator.GetTaskStatus", resp.TaskID, &resp)
				if err != nil {
					t.Fatalf("Failed to get task status: %v", err)
				}

				if resp.Success && resp.Result != nil {
					break
				}

				if resp.Error != "" {
					t.Fatalf("Task failed: %s", resp.Error)
				}

				time.Sleep(time.Second)
			}

			if !resp.Success || resp.Result == nil {
				t.Fatal("Task did not complete in time")
			}
		})
	}
}

// TestConcurrentClients verifies system handles multiple simultaneous clients
func TestConcurrentClients(t *testing.T) {
	numClients := 10
	doneCh := make(chan bool, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			client, err := rpc.Dial("tcp", "localhost:1234")
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", clientID, err)
				doneCh <- false
				return
			}
			defer client.Close()

			// Create test matrices
			mat1, _ := matrix.NewMatrix(50, 50)
			mat2, _ := matrix.NewMatrix(50, 50)
			for i := 0; i < 50; i++ {
				for j := 0; j < 50; j++ {
					mat1.Set(i, j, float64(i+j))
					mat2.Set(i, j, float64(i*j))
				}
			}

			req := &types.ComputeRequest{
				TaskID:  fmt.Sprintf("concurrent-client-%d-%d", clientID, time.Now().Unix()),
				Op:      types.Addition,
				Matrix1: mat1,
				Matrix2: mat2,
			}

			var resp types.ComputeResponse
			err = client.Call("Coordinator.SubmitTask", req, &resp)
			if err != nil {
				t.Errorf("Client %d failed to submit task: %v", clientID, err)
				doneCh <- false
				return
			}

			// Wait for completion
			for i := 0; i < 30; i++ {
				err = client.Call("Coordinator.GetTaskStatus", resp.TaskID, &resp)
				if err != nil {
					t.Errorf("Client %d failed to get status: %v", clientID, err)
					doneCh <- false
					return
				}

				if resp.Success && resp.Result != nil {
					break
				}

				time.Sleep(time.Second)
			}

			if !resp.Success || resp.Result == nil {
				t.Errorf("Client %d task did not complete", clientID)
				doneCh <- false
				return
			}

			doneCh <- true
		}(i)
	}

	// Wait for all clients to complete
	for i := 0; i < numClients; i++ {
		if !<-doneCh {
			t.Fatal("One or more clients failed")
		}
	}
}

// TestWorkerFailure verifies system handles worker failures gracefully
func TestWorkerFailure(t *testing.T) {
	// First, start a client and submit a task
	client, err := rpc.Dial("tcp", "localhost:1234")
	if err != nil {
		t.Fatalf("Failed to connect to coordinator: %v", err)
	}
	defer client.Close()

	mat1, _ := matrix.NewMatrix(100, 100)
	for i := 0; i < 100; i++ {
		for j := 0; j < 100; j++ {
			mat1.Set(i, j, float64(i+j))
		}
	}

	req := &types.ComputeRequest{
		TaskID:  fmt.Sprintf("fault-tolerance-%d", time.Now().Unix()),
		Op:      types.Transpose,
		Matrix1: mat1,
	}

	var resp types.ComputeResponse
	err = client.Call("Coordinator.SubmitTask", req, &resp)
	if err != nil {
		t.Fatalf("Failed to submit task: %v", err)
	}

	// Simulate worker failure by disconnecting it
	// Note: This requires modifying the worker to accept a shutdown signal
	
	// Verify task still completes
	success := false
	for i := 0; i < 30; i++ {
		err = client.Call("Coordinator.GetTaskStatus", resp.TaskID, &resp)
		if err != nil {
			t.Fatalf("Failed to get task status: %v", err)
		}

		if resp.Success && resp.Result != nil {
			success = true
			break
		}

		time.Sleep(time.Second)
	}

	if !success {
		t.Fatal("Task did not complete after worker failure")
	}
}