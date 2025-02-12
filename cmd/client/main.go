// Example client code to perform matrix multiplication and addition

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
    coordinatorAddr := flag.String("coordinator", "localhost:1234", "Coordinator address")
    useTLS := flag.Bool("tls", false, "Use TLS for secure communication")
    operation := flag.String("op", "multiply", "Operation to perform: multiply, add")
    retryCount := flag.Int("retry", 3, "Number of connection retry attempts")
    flag.Parse()

    // Initialize random seed
    rand.Seed(time.Now().UnixNano())

    // Connect to the coordinator with retry logic
    var client *rpc.Client
    var err error

    for attempt := 1; attempt <= *retryCount; attempt++ {
        if *useTLS {
            config := &tls.Config{
                InsecureSkipVerify: true,
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

    // Create two 3x3 test matrices
    mat1, err := matrix.NewMatrix(3, 3)
    if err != nil {
        log.Fatal("Failed to create first matrix:", err)
    }

    mat2, err := matrix.NewMatrix(3, 3)
    if err != nil {
        log.Fatal("Failed to create second matrix:", err)
    }

    // Set values for both matrices
    for i := 0; i < 3; i++ {
        for j := 0; j < 3; j++ {
            // Generate random values between 1 and 10 for easier verification
            val1 := float64(rand.Intn(10)) + 1
            val2 := float64(rand.Intn(10)) + 1
            mat1.Set(i, j, val1)
            mat2.Set(i, j, val2)
        }
    }

    log.Printf("\n=== Client creating computation request ===\n")
    log.Printf("First Matrix:\n%s", mat1)
    log.Printf("Second Matrix:\n%s", mat2)

    // Determine operation type
    var op types.Operation
    switch *operation {
    case "multiply":
        op = types.Multiplication
        log.Printf("Operation: Multiplication\n")
    case "add":
        op = types.Addition
        log.Printf("Operation: Addition\n")
    default:
        log.Fatalf("Unknown operation: %s", *operation)
    }

    // Create compute request
    req := &types.ComputeRequest{
        TaskID:  fmt.Sprintf("task-%d", time.Now().UnixNano()),
        Op:      op,
        Matrix1: mat1,
        Matrix2: mat2,
    }

    var resp types.ComputeResponse

    // Submit the task
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