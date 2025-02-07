// cmd/client/main.go

package main

import (
    "fmt"
    "log"
    "matrix-compute/pkg/matrix"
    "matrix-compute/pkg/types"
    "net/rpc"
    "time"
)

func main() {
    // Connect to the coordinator
    client, err := rpc.Dial("tcp", "localhost:1234")
    if err != nil {
        log.Fatal("Failed to connect to coordinator:", err)
    }
    defer client.Close()

    // Create a test matrix
    mat1, err := matrix.NewMatrix(2, 2)
    if err != nil {
        log.Fatal("Failed to create matrix:", err)
    }

    // Set some values
    mat1.Set(0, 0, 1.0)
    mat1.Set(0, 1, 2.0)
    mat1.Set(1, 0, 3.0)
    mat1.Set(1, 1, 4.0)

    fmt.Println("Input Matrix:")
    fmt.Println(mat1)

    // Create a compute request
    req := &types.ComputeRequest{
        TaskID:  fmt.Sprintf("task-%d", time.Now().Unix()),
        Op:      types.Transpose,
        Matrix1: mat1,
    }

    var resp types.ComputeResponse

    // Submit the task
    err = client.Call("Coordinator.SubmitTask", req, &resp)
    if err != nil {
        log.Fatal("Error submitting task:", err)
    }

    fmt.Printf("Task submitted successfully. Task ID: %s\n", resp.TaskID)

    // Check task status
    maxAttempts := 10
    for i := 0; i < maxAttempts; i++ {
        err = client.Call("Coordinator.GetTaskStatus", resp.TaskID, &resp)
        if err != nil {
            log.Fatal("Error checking task status:", err)
        }

        if resp.Success && resp.Result != nil {
            fmt.Println("Task completed!")
            fmt.Println("Result:")
            fmt.Println(resp.Result)
            break
        }

        if resp.Error != "" {
            fmt.Printf("Task failed: %s\n", resp.Error)
            break
        }

        fmt.Printf("Task still processing... (attempt %d/%d)\n", i+1, maxAttempts)
        time.Sleep(time.Second)
    }
}