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
    log.Printf("\n=== Worker %s processing task %s ===\n", w.ID, task.ID)
    log.Printf("Operation type: %d\n", task.Op)

    // Validate input matrices
    if task.Matrix1 == nil {
        resp.Error = "first matrix is nil"
        resp.Success = false
        return nil
    }

    if task.Matrix1.Rows <= 0 || task.Matrix1.Cols <= 0 {
        resp.Error = "invalid matrix dimensions"
        resp.Success = false
        return nil
    }

    if task.Op != types.Transpose {
        if task.Matrix2 == nil {
            resp.Error = "second matrix is required for this operation"
            resp.Success = false
            return nil
        }
        if task.Matrix2.Rows <= 0 || task.Matrix2.Cols <= 0 {
            resp.Error = "invalid second matrix dimensions"
            resp.Success = false
            return nil
        }
    }

    var err error
    resp.TaskID = task.ID

    log.Printf("\nInput Matrix:\n%s", task.Matrix1)
    if task.Matrix2 != nil {
        log.Printf("Second Matrix:\n%s", task.Matrix2)
    }

    switch task.Op {
    case types.Addition:
        resp.Result, err = w.Add(task.Matrix1, task.Matrix2)
    case types.Transpose:
        resp.Result, err = w.Transpose(task.Matrix1)
    case types.Multiplication:
        resp.Result, err = w.Multiply(task.Matrix1, task.Matrix2)
    default:
        err = fmt.Errorf("unsupported operation: %d", task.Op)
    }

    if err != nil {
        resp.Error = err.Error()
        resp.Success = false
        return nil
    }

    w.taskCount++
    resp.Success = true
    log.Printf("Task %s completed successfully\n", task.ID)
    log.Printf("=== End of task processing ===\n")

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
        return nil, fmt.Errorf("matrix dimensions do not match for addition: (%dx%d) and (%dx%d)",
            m1.Rows, m1.Cols, m2.Rows, m2.Cols)
    }

    result, err := matrix.NewMatrix(m1.Rows, m1.Cols)
    if err != nil {
        return nil, fmt.Errorf("failed to create result matrix: %v", err)
    }

    for i := 0; i < m1.Rows; i++ {
        for j := 0; j < m1.Cols; j++ {
            val1, err1 := m1.Get(i, j)
            val2, err2 := m2.Get(i, j)
            if err1 != nil || err2 != nil {
                return nil, fmt.Errorf("error accessing matrix elements: %v, %v", err1, err2)
            }
            err = result.Set(i, j, val1+val2)
            if err != nil {
                return nil, fmt.Errorf("error setting result matrix element: %v", err)
            }
        }
    }

    return result, nil
}

func (w *Worker) Transpose(m *matrix.Matrix) (*matrix.Matrix, error) {
    if m == nil {
        return nil, fmt.Errorf("cannot transpose nil matrix")
    }

    result, err := matrix.NewMatrix(m.Cols, m.Rows)
    if err != nil {
        return nil, fmt.Errorf("failed to create result matrix: %v", err)
    }

    for i := 0; i < m.Rows; i++ {
        for j := 0; j < m.Cols; j++ {
            val, err := m.Get(i, j)
            if err != nil {
                return nil, fmt.Errorf("error accessing matrix element: %v", err)
            }
            err = result.Set(j, i, val)
            if err != nil {
                return nil, fmt.Errorf("error setting result matrix element: %v", err)
            }
        }
    }

    return result, nil
}

func (w *Worker) Multiply(m1, m2 *matrix.Matrix) (*matrix.Matrix, error) {
    if m1.Cols != m2.Rows {
        return nil, fmt.Errorf("invalid dimensions for multiplication: (%dx%d) and (%dx%d)",
            m1.Rows, m1.Cols, m2.Rows, m2.Cols)
    }

    result, err := matrix.NewMatrix(m1.Rows, m2.Cols)
    if err != nil {
        return nil, fmt.Errorf("failed to create result matrix: %v", err)
    }

    for i := 0; i < m1.Rows; i++ {
        for j := 0; j < m2.Cols; j++ {
            sum := 0.0
            for k := 0; k < m1.Cols; k++ {
                val1, err1 := m1.Get(i, k)
                val2, err2 := m2.Get(k, j)
                if err1 != nil || err2 != nil {
                    return nil, fmt.Errorf("error accessing matrix elements: %v, %v", err1, err2)
                }
                sum += val1 * val2
            }
            err = result.Set(i, j, sum)
            if err != nil {
                return nil, fmt.Errorf("error setting result matrix element: %v", err)
            }
        }
    }

    return result, nil
}

// In cmd/worker/main.go

func main() {
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

        // Create TLS configuration for listener with updated settings
        config := &tls.Config{
            Certificates:       []tls.Certificate{tlsCert},
            InsecureSkipVerify: true,
            ClientAuth:         tls.NoClientCert,  // Changed this
            MinVersion:         tls.VersionTLS12,
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

    log.Printf("Worker started on %s with TLS=%v\n", worker.ID, *useTLS)

    // Connect to coordinator with TLS
    var coordinatorClient *rpc.Client
    if *useTLS {
        config := &tls.Config{
            InsecureSkipVerify: true,
            Certificates:       []tls.Certificate{tlsCert},
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

    // Custom connection handler for better error logging
    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Printf("Accept error: %v\n", err)
            continue
        }

        go func(conn net.Conn) {
            defer conn.Close()
            
            if *useTLS {
                tlsConn := conn.(*tls.Conn)
                if err := tlsConn.Handshake(); err != nil {
                    log.Printf("TLS handshake failed: %v\n", err)
                    return
                }
                log.Printf("TLS handshake successful with %s\n", tlsConn.RemoteAddr())
            }
            
            server.ServeConn(conn)
        }(conn)
    }
}
