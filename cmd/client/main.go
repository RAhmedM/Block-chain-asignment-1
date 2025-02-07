// cmd/client/main.go

package main

import (
    "crypto/tls"
    "flag"
    "fmt"
    "log"
    "math/rand"
    "matrix-compute/pkg/matrix"
    "matrix-compute/pkg/types"
    "net/rpc"
    "time"
)

func main() {
    // Initialize random seed
    rand.Seed(time.Now().UnixNano())

    // Command line flags
    coordinatorAddr := flag.String("coordinator", "localhost:1234", "Coordinator address")
    useTLS := flag.Bool("tls", false, "Use TLS for secure communication")
    retryCount := flag.Int("retry", 3, "Number of connection retry attempts")
    flag.Parse()

    // Connect to the coordinator with retry logic
    var client *rpc.Client
    var err error

    for attempt := 1; attempt <= *retryCount; attempt++ {
        if *useTLS {
            config := &tls.Config{
                InsecureSkipVerify: true, // For self-signed certificates
            }
            var conn *tls.Conn
            conn, err = tls.Dial("tcp", *coordinatorAddr, config)
            if err == nil {
                client = rpc.NewClient(conn)
                break
            }
        } else {
            client, err = rpc.Dial("tcp", *coordinatorAddr)
            if err == nil {
                break
            }
        }
        
        log.Printf("Connection attempt %d/%d failed: %v\n", attempt, *retryCount, err)
        if attempt < *retryCount {
            time.Sleep(time.Second * time.Duration(attempt))
        }
    }

    if err != nil {
        log.Fatalf("Failed to connect after %d attempts: %v", *retryCount, err)
    }
    defer client.Close()

    log.Printf("Successfully connected to coordinator at %s\n", *coordinatorAddr)

    // Create a 10x10 test matrix
    mat1, err := matrix.NewMatrix(10, 10)
    if err != nil {
        log.Fatal("Failed to create matrix:", err)
    }

    // Set values for the 10x10 matrix
    for i := 0; i < 10; i++ {
        for j := 0; j < 10; j++ {
            // Generate random float64 between 0 and 100
            randomValue := rand.Float64() * 100
            if err := mat1.Set(i, j, randomValue); err != nil {
                log.Fatal("Failed to set matrix value:", err)
            }
        }
    }

    log.Printf("\n=== Client creating computation request ===\n")
    log.Printf("Matrix to be transposed:\n%s", mat1)

    // Create a compute request
    req := &types.ComputeRequest{
        TaskID:  fmt.Sprintf("task-%d", time.Now().UnixNano()),
        Op:      types.Transpose,
        Matrix1: mat1,
    }

    var resp types.ComputeResponse

    // Submit the task with timeout
    done := make(chan error, 1)
    go func() {
        done <- client.Call("Coordinator.SubmitTask", req, &resp)
    }()

    select {
    case err := <-done:
        if err != nil {
            log.Fatal("Error submitting task:", err)
        }
    case <-time.After(10 * time.Second):
        log.Fatal("Timeout submitting task")
    }

    log.Printf("Task submitted successfully. Task ID: %s\n", resp.TaskID)
    log.Printf("Waiting for result...\n")

    // Check task status with exponential backoff
    maxAttempts := 10
    for i := 0; i < maxAttempts; i++ {
        err = client.Call("Coordinator.GetTaskStatus", resp.TaskID, &resp)
        if err != nil {
            log.Printf("Error checking task status: %v\n", err)
            time.Sleep(time.Second * time.Duration(1<<uint(i)))
            continue
        }

        if resp.Success && resp.Result != nil {
            log.Printf("\n=== Final result received ===\n")
            log.Printf("Task completed successfully\n")
            log.Printf("Result matrix:\n%s", resp.Result)
            break
        }

        if resp.Error != "" {
            log.Printf("Task failed with error: %s\n", resp.Error)
            break
        }

        log.Printf("Task still processing... (attempt %d/%d)\n", i+1, maxAttempts)
        time.Sleep(time.Second * time.Duration(1<<uint(i)))
    }
}