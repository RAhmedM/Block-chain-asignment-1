// cmd/worker/main.go

package main

import (
    "flag"
    "fmt"
    "log"
    "matrix-compute/pkg/matrix"
    "matrix-compute/pkg/types"
    "net"
    "net/rpc"
    "time"
)

// Worker handles matrix computations
type Worker struct {
    ID        string
    taskCount int
}

// ExecuteTask performs the requested matrix operation
func (w *Worker) ExecuteTask(task *types.Task, resp *types.ComputeResponse) error {
    log.Printf("Worker %s received task %s\n", w.ID, task.ID)
    var err error
    resp.TaskID = task.ID

    switch task.Op {
    case types.Addition:
        resp.Result, err = w.Add(task.Matrix1, task.Matrix2)
    case types.Transpose:
        resp.Result, err = w.Transpose(task.Matrix1)
    case types.Multiplication:
        resp.Result, err = w.Multiply(task.Matrix1, task.Matrix2)
    }

    if err != nil {
        resp.Error = err.Error()
        resp.Success = false
        return nil
    }

    w.taskCount++
    resp.Success = true
    log.Printf("Worker %s completed task %s\n", w.ID, task.ID)
    return nil
}

// GetStatus returns the current status of the worker
func (w *Worker) GetStatus(_ *struct{}, status *types.WorkerStatus) error {
    status.ID = w.ID
    status.Available = true
    status.TaskCount = w.taskCount
    status.LastUpdated = time.Now().Unix()
    return nil
}

// Add performs matrix addition
func (w *Worker) Add(m1, m2 *matrix.Matrix) (*matrix.Matrix, error) {
    if m1.Rows != m2.Rows || m1.Cols != m2.Cols {
        return nil, types.MatrixError("invalid dimensions for addition")
    }

    result, err := matrix.NewMatrix(m1.Rows, m1.Cols)
    if err != nil {
        return nil, err
    }

    for i := 0; i < m1.Rows; i++ {
        for j := 0; j < m1.Cols; j++ {
            val1, _ := m1.Get(i, j)
            val2, _ := m2.Get(i, j)
            result.Set(i, j, val1+val2)
        }
    }

    return result, nil
}

// Transpose performs matrix transposition
func (w *Worker) Transpose(m *matrix.Matrix) (*matrix.Matrix, error) {
    result, err := matrix.NewMatrix(m.Cols, m.Rows)
    if err != nil {
        return nil, err
    }

    for i := 0; i < m.Rows; i++ {
        for j := 0; j < m.Cols; j++ {
            val, _ := m.Get(i, j)
            result.Set(j, i, val)
        }
    }

    return result, nil
}

// Multiply performs matrix multiplication
func (w *Worker) Multiply(m1, m2 *matrix.Matrix) (*matrix.Matrix, error) {
    if m1.Cols != m2.Rows {
        return nil, types.MatrixError("invalid dimensions for multiplication")
    }

    result, err := matrix.NewMatrix(m1.Rows, m2.Cols)
    if err != nil {
        return nil, err
    }

    for i := 0; i < m1.Rows; i++ {
        for j := 0; j < m2.Cols; j++ {
            sum := 0.0
            for k := 0; k < m1.Cols; k++ {
                val1, _ := m1.Get(i, k)
                val2, _ := m2.Get(k, j)
                sum += val1 * val2
            }
            result.Set(i, j, sum)
        }
    }

    return result, nil
}

func main() {
    // Command line flags for worker configuration
    port := flag.Int("port", 0, "Port to listen on (0 for random)")
    coordinatorAddr := flag.String("coordinator", "localhost:1234", "Coordinator address")
    flag.Parse()

    // Create worker instance
    worker := &Worker{}

    // Register RPC service
    server := rpc.NewServer()
    err := server.Register(worker)
    if err != nil {
        log.Fatal("Failed to register worker:", err)
    }

    // Start listening for coordinator connections
    listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
    if err != nil {
        log.Fatal("Failed to start listener:", err)
    }

    // Get the actual address we're listening on
    addr := listener.Addr().(*net.TCPAddr)
    worker.ID = fmt.Sprintf("localhost:%d", addr.Port)

    log.Printf("Worker started on %s\n", worker.ID)

    // Register with coordinator
    client, err := rpc.Dial("tcp", *coordinatorAddr)
    if err != nil {
        log.Fatal("Failed to connect to coordinator:", err)
    }

    // Create and send worker status
    status := &types.WorkerStatus{
        ID:          worker.ID,
        Available:   true,
        TaskCount:   0,
        LastUpdated: time.Now().Unix(),
    }

    var reply bool
    err = client.Call("Coordinator.RegisterWorker", status, &reply)
    if err != nil {
        log.Fatal("Failed to register with coordinator:", err)
    }
    client.Close()

    if !reply {
        log.Fatal("Coordinator rejected worker registration")
    }

    log.Printf("Successfully registered with coordinator")

    // Start serving requests
    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Println("Accept error:", err)
            continue
        }
        go server.ServeConn(conn)
    }
}