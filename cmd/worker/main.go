// cmd/worker/main.go

package main

import (
    "crypto/tls"
    "flag"
    "fmt"
    "log"
    "matrix-compute/pkg/matrix"
    "matrix-compute/pkg/types"
    tlsutil "matrix-compute/pkg/tls"
    "net"
    "net/rpc"
    "time"
)

// Worker handles matrix computations
type Worker struct {
    ID        string
    taskCount int
    client    *rpc.Client
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

    // Update coordinator about our status
    w.reportStatus()
    
    return nil
}

// reportStatus sends the current worker status to the coordinator
func (w *Worker) reportStatus() {
    status := &types.WorkerStatus{
        ID:          w.ID,
        Available:   true,
        TaskCount:   w.taskCount,
        LastUpdated: time.Now().Unix(),
    }

    var reply bool
    err := w.client.Call("Coordinator.UpdateWorkerStatus", status, &reply)
    if err != nil {
        log.Printf("Failed to update coordinator: %v\n", err)
    }
}

// startHealthReporting periodically sends health updates to coordinator
func (w *Worker) startHealthReporting() {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        for range ticker.C {
            w.reportStatus()
        }
    }()
}

// Matrix operation implementations
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
    useTLS := flag.Bool("tls", false, "Use TLS for secure communication")
    flag.Parse()

    // Create worker instance
    worker := &Worker{}

    // Register RPC service
    server := rpc.NewServer()
    err := server.RegisterName("Worker", worker)
    if err != nil {
        log.Fatal("Failed to register worker:", err)
    }

    var listener net.Listener
    var tlsCert tls.Certificate
    
    if *useTLS {
        // Generate TLS certificate
        var err error
        tlsCert, err = tlsutil.GenerateCertificate("localhost")
        if err != nil {
            log.Fatal("Failed to generate TLS certificate:", err)
        }

        // Create TLS configuration for listener
        config := &tls.Config{
            Certificates: []tls.Certificate{tlsCert},
            InsecureSkipVerify: true, // For self-signed certificates
        }

        // Start TLS listener
        listener, err = tls.Listen("tcp", fmt.Sprintf(":%d", *port), config)
        if err != nil {
            log.Fatal("Failed to start TLS listener:", err)
        }
    } else {
        listener, err = net.Listen("tcp", fmt.Sprintf(":%d", *port))
        if err != nil {
            log.Fatal("Failed to start listener:", err)
        }
    }

    // Get the actual address we're listening on
    addr := listener.Addr().(*net.TCPAddr)
    worker.ID = fmt.Sprintf("localhost:%d", addr.Port)

    log.Printf("Worker started on %s\n", worker.ID)

    // Connect to coordinator
    var coordinatorClient *rpc.Client
    if *useTLS {
        config := &tls.Config{
            InsecureSkipVerify: true, // For self-signed certificates
        }
        conn, err := tls.Dial("tcp", *coordinatorAddr, config)
        if err != nil {
            log.Fatal("Failed to connect to coordinator:", err)
        }
        coordinatorClient = rpc.NewClient(conn)
    } else {
        coordinatorClient, err = rpc.Dial("tcp", *coordinatorAddr)
        if err != nil {
            log.Fatal("Failed to connect to coordinator:", err)
        }
    }
    worker.client = coordinatorClient

    // Register with coordinator
    status := &types.WorkerStatus{
        ID:          worker.ID,
        Available:   true,
        TaskCount:   0,
        LastUpdated: time.Now().Unix(),
    }

    var reply bool
    err = coordinatorClient.Call("Coordinator.RegisterWorker", status, &reply)
    if err != nil {
        log.Fatal("Failed to register with coordinator:", err)
    }

    if !reply {
        log.Fatal("Coordinator rejected worker registration")
    }

    log.Printf("Successfully registered with coordinator")

    // Start health reporting
    worker.startHealthReporting()

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