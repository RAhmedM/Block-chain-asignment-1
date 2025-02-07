// pkg/matrix/matrix_test.go

package matrix_test

import (
    "context"
    "fmt"
    "matrix-compute/pkg/matrix"
    "matrix-compute/pkg/types"
    "net/rpc"
    "os/exec"
    "strings"
    "sync"
    "testing"
    "time"
)
// TestHelper encapsulates common test setup and teardown
type TestHelper struct {
    coordinator *exec.Cmd
    workers    []*exec.Cmd
    client     *rpc.Client
    t          *testing.T
}

func NewTestHelper(t *testing.T) *TestHelper {
    return &TestHelper{t: t}
}

// StartSystem starts the coordinator and specified number of workers
func (h *TestHelper) StartSystem(numWorkers int) {
    // Start coordinator
    h.coordinator = exec.Command("go", "run", "../../cmd/coordinator/main.go", "-v")
    if err := h.coordinator.Start(); err != nil {
        h.t.Fatalf("Failed to start coordinator: %v", err)
    }

    // Wait for coordinator to be ready
    time.Sleep(2 * time.Second)

    // Start workers
    for i := 0; i < numWorkers; i++ {
        port := 2001 + i
        worker := exec.Command("go", "run", "../../cmd/worker/main.go", "-port", fmt.Sprintf("%d", port))
        if err := worker.Start(); err != nil {
            h.t.Fatalf("Failed to start worker %d: %v", i, err)
        }
        h.workers = append(h.workers, worker)
    }

    // Wait for workers to register
    time.Sleep(2 * time.Second)

    // Connect client
    var err error
    h.client, err = rpc.Dial("tcp", "localhost:1234")
    if err != nil {
        h.t.Fatalf("Failed to connect client: %v", err)
    }
}

// Cleanup stops all processes
func (h *TestHelper) Cleanup() {
    if h.client != nil {
        h.client.Close()
    }
    
    for _, worker := range h.workers {
        if worker != nil && worker.Process != nil {
            worker.Process.Kill()
        }
    }
    
    if h.coordinator != nil && h.coordinator.Process != nil {
        h.coordinator.Process.Kill()
    }
}

// WaitForTask waits for task completion with timeout
func (h *TestHelper) WaitForTask(taskID string, timeout time.Duration) (*types.ComputeResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil, fmt.Errorf("timeout waiting for task completion")
        case <-ticker.C:
            var resp types.ComputeResponse
            err := h.client.Call("Coordinator.GetTaskStatus", taskID, &resp)
            if err != nil {
                return nil, err
            }
            if resp.Success && resp.Result != nil {
                return &resp, nil
            }
            if resp.Error != "" {
                return &resp, fmt.Errorf("task failed: %s", resp.Error)
            }
        }
    }
}

func TestBasicMatrixOperations(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    helper.StartSystem(3)

    tests := []struct {
        name     string
        op       types.Operation
        rows     int
        cols     int
        validate func(*matrix.Matrix) error
    }{
        {
            name: "Small Matrix Addition",
            op:   types.Addition,
            rows: 2,
            cols: 2,
            validate: func(result *matrix.Matrix) error {
                expected := [][]float64{{2, 4}, {6, 8}}
                for i := 0; i < 2; i++ {
                    for j := 0; j < 2; j++ {
                        val, _ := result.Get(i, j)
                        if val != expected[i][j] {
                            return fmt.Errorf("expected %v at [%d,%d], got %v", expected[i][j], i, j, val)
                        }
                    }
                }
                return nil
            },
        },
        {
            name: "Matrix Transpose",
            op:   types.Transpose,
            rows: 2,
            cols: 3,
            validate: func(result *matrix.Matrix) error {
                if result.Rows != 3 || result.Cols != 2 {
                    return fmt.Errorf("expected dimensions 3x2, got %dx%d", result.Rows, result.Cols)
                }
                return nil
            },
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            mat1, err := matrix.NewMatrix(tc.rows, tc.cols)
            if err != nil {
                t.Fatalf("Failed to create first matrix: %v", err)
            }

            // Initialize first matrix
            for i := 0; i < tc.rows; i++ {
                for j := 0; j < tc.cols; j++ {
                    if err := mat1.Set(i, j, float64(i+j+1)); err != nil {
                        t.Fatalf("Failed to set matrix value: %v", err)
                    }
                }
            }

            var mat2 *matrix.Matrix
            if tc.op == types.Addition || tc.op == types.Multiplication {
                mat2, err = matrix.NewMatrix(tc.rows, tc.cols)
                if err != nil {
                    t.Fatalf("Failed to create second matrix: %v", err)
                }
                for i := 0; i < tc.rows; i++ {
                    for j := 0; j < tc.cols; j++ {
                        if err := mat2.Set(i, j, float64(i+j+1)); err != nil {
                            t.Fatalf("Failed to set matrix value: %v", err)
                        }
                    }
                }
            }

            req := &types.ComputeRequest{
                TaskID:  fmt.Sprintf("test-%s-%d", tc.name, time.Now().Unix()),
                Op:      tc.op,
                Matrix1: mat1,
                Matrix2: mat2,
            }

            var resp types.ComputeResponse
            err = helper.client.Call("Coordinator.SubmitTask", req, &resp)
            if err != nil {
                t.Fatalf("Failed to submit task: %v", err)
            }

            result, err := helper.WaitForTask(resp.TaskID, 10*time.Second)
            if err != nil {
                t.Fatalf("Task failed: %v", err)
            }

            if err := tc.validate(result.Result); err != nil {
                t.Errorf("Validation failed: %v", err)
            }
        })
    }
}

func TestEdgeCases(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    helper.StartSystem(3)

    tests := []struct {
        name          string
        setupMatrix   func() (*matrix.Matrix, *matrix.Matrix)
        op            types.Operation
        expectedError string
    }{
        {
            name: "Zero Size Matrix",
            setupMatrix: func() (*matrix.Matrix, *matrix.Matrix) {
                mat, _ := matrix.NewMatrix(0, 0)
                return mat, nil
            },
            op:            types.Transpose,
            expectedError: "invalid matrix dimensions",
        },
        {
            name: "Nil Matrix",
            setupMatrix: func() (*matrix.Matrix, *matrix.Matrix) {
                return nil, nil
            },
            op:            types.Transpose,
            expectedError: "first matrix is nil",
        },
        {
            name: "Mismatched Dimensions for Addition",
            setupMatrix: func() (*matrix.Matrix, *matrix.Matrix) {
                mat1, _ := matrix.NewMatrix(2, 3)
                mat2, _ := matrix.NewMatrix(3, 2)
                return mat1, mat2
            },
            op:            types.Addition,
            expectedError: "matrix dimensions do not match",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            mat1, mat2 := tc.setupMatrix()
            req := &types.ComputeRequest{
                TaskID:  fmt.Sprintf("edge-case-%s-%d", tc.name, time.Now().Unix()),
                Op:      tc.op,
                Matrix1: mat1,
                Matrix2: mat2,
            }

            var resp types.ComputeResponse
            err := helper.client.Call("Coordinator.SubmitTask", req, &resp)
            if err != nil {
                // Some errors might be caught at submission time
                if !strings.Contains(err.Error(), tc.expectedError) {
                    t.Errorf("Expected error containing %q, got %q", tc.expectedError, err.Error())
                }
                return
            }

            // Wait for task completion
            result, err := helper.WaitForTask(resp.TaskID, 5*time.Second)
            if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
                t.Errorf("Expected error containing %q, got %v", tc.expectedError, err)
            }

            if result != nil && result.Success {
                t.Error("Expected task to fail, but it succeeded")
            }
        })
    }
}


func TestConcurrentOperations(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    helper.StartSystem(5)

    numOperations := 5
    var wg sync.WaitGroup
    errors := make(chan error, numOperations)

    for i := 0; i < numOperations; i++ {
        wg.Add(1)
        go func(opNum int) {
            defer wg.Done()

            size := 10
            mat1, _ := matrix.NewMatrix(size, size)
            mat2, _ := matrix.NewMatrix(size, size)

            for i := 0; i < size; i++ {
                for j := 0; j < size; j++ {
                    mat1.Set(i, j, float64(i+j))
                    mat2.Set(i, j, float64(i*j))
                }
            }

            req := &types.ComputeRequest{
                TaskID:  fmt.Sprintf("concurrent-op-%d-%d", opNum, time.Now().Unix()),
                Op:      types.Addition,
                Matrix1: mat1,
                Matrix2: mat2,
            }

            var resp types.ComputeResponse
            if err := helper.client.Call("Coordinator.SubmitTask", req, &resp); err != nil {
                errors <- fmt.Errorf("operation %d failed to submit: %v", opNum, err)
                return
            }

            if _, err := helper.WaitForTask(resp.TaskID, 30*time.Second); err != nil {
                errors <- fmt.Errorf("operation %d failed: %v", opNum, err)
                return
            }
        }(i)
    }

    wg.Wait()
    close(errors)

    for err := range errors {
        t.Errorf("Concurrent operation error: %v", err)
    }
}