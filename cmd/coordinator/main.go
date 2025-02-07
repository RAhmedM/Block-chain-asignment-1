// cmd/coordinator/main.go

package main

import (
    "crypto/tls"
    "flag"
    "log"
    "matrix-compute/pkg/types"
    tlsutil "matrix-compute/pkg/tls"
    "net"
    "net/rpc"
    "sync"
    "time"
)

const (
    workerTimeout     = 10 * time.Second
    healthCheckPeriod = 5 * time.Second
    maxRetries        = 3
)

// CoordinatorService handles task distribution and worker management
type CoordinatorService struct {
    tasks           map[string]*types.Task
    workers         map[string]*types.WorkerStatus
    taskQueue       []string
    retryCount      map[string]int
    taskAssignments map[string]string // taskID -> workerID
    mu             sync.Mutex
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator() *CoordinatorService {
    c := &CoordinatorService{
        tasks:           make(map[string]*types.Task),
        workers:         make(map[string]*types.WorkerStatus),
        taskQueue:       make([]string, 0),
        retryCount:      make(map[string]int),
        taskAssignments: make(map[string]string),
    }
    go c.processTaskQueue()
    go c.monitorWorkerHealth()
    return c
}

// RegisterWorker registers a new worker with the coordinator
func (c *CoordinatorService) RegisterWorker(status *types.WorkerStatus, reply *bool) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.workers[status.ID] = status
    log.Printf("Worker %s registered\n", status.ID)
    *reply = true
    return nil
}

// SubmitTask handles new task submissions from clients
func (c *CoordinatorService) SubmitTask(req *types.ComputeRequest, resp *types.ComputeResponse) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    // Create new task
    task := &types.Task{
        ID:        req.TaskID,
        Op:        req.Op,
        Matrix1:   req.Matrix1,
        Matrix2:   req.Matrix2,
        Completed: false,
    }

    // Add to task map and queue
    c.tasks[req.TaskID] = task
    c.taskQueue = append(c.taskQueue, req.TaskID)

    // Set initial response
    resp.TaskID = req.TaskID
    resp.Success = true

    return nil
}

// GetTaskStatus returns the current status of a task
func (c *CoordinatorService) GetTaskStatus(taskID string, resp *types.ComputeResponse) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    task, exists := c.tasks[taskID]
    if !exists {
        resp.Success = false
        resp.Error = "task not found"
        return nil
    }

    resp.TaskID = taskID
    resp.Result = task.Result
    resp.Success = task.Completed
    resp.Error = task.Error

    return nil
}

// UpdateWorkerStatus updates a worker's status
func (c *CoordinatorService) UpdateWorkerStatus(status *types.WorkerStatus, reply *bool) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    if worker, exists := c.workers[status.ID]; exists {
        worker.Available = status.Available
        worker.TaskCount = status.TaskCount
        worker.LastUpdated = time.Now().Unix()
    }
    *reply = true
    return nil
}

// monitorWorkerHealth periodically checks worker health
func (c *CoordinatorService) monitorWorkerHealth() {
    for {
        time.Sleep(healthCheckPeriod)
        c.mu.Lock()
        now := time.Now().Unix()

        for id, worker := range c.workers {
            if now-worker.LastUpdated > int64(workerTimeout.Seconds()) {
                log.Printf("Worker %s appears to be dead, removing...", id)
                delete(c.workers, id)

                // Reassign tasks from failed worker
                for taskID, workerID := range c.taskAssignments {
                    if workerID == id {
                        if c.retryCount[taskID] < maxRetries {
                            c.taskQueue = append(c.taskQueue, taskID)
                            c.retryCount[taskID]++
                            delete(c.taskAssignments, taskID)
                            log.Printf("Reassigning task %s (attempt %d/%d)", taskID, c.retryCount[taskID], maxRetries)
                        } else {
                            log.Printf("Task %s failed after %d attempts", taskID, maxRetries)
                            if task, exists := c.tasks[taskID]; exists {
                                task.Error = "maximum retry attempts exceeded"
                                task.Completed = true
                            }
                        }
                    }
                }
            }
        }
        c.mu.Unlock()
    }
}

// getLeastBusyWorker returns the worker with the lowest load
func (c *CoordinatorService) getLeastBusyWorker() *types.WorkerStatus {
    var selected *types.WorkerStatus
    minLoad := float64(-1)

    now := time.Now().Unix()
    for _, worker := range c.workers {
        if !worker.Available || now-worker.LastUpdated > int64(workerTimeout.Seconds()) {
            continue
        }

        // Calculate load score based on task count and recent activity
        loadScore := float64(worker.TaskCount)
        if selected == nil || loadScore < minLoad {
            selected = worker
            minLoad = loadScore
        }
    }

    return selected
}

// processTaskQueue continuously processes tasks in the queue
func (c *CoordinatorService) processTaskQueue() {
    for {
        c.mu.Lock()
        if len(c.taskQueue) > 0 {
            taskID := c.taskQueue[0]
            if worker := c.getLeastBusyWorker(); worker != nil {
                task := c.tasks[taskID]
                c.taskQueue = c.taskQueue[1:]
                c.taskAssignments[taskID] = worker.ID
                go c.assignTaskToWorker(task, worker)
            }
        }
        c.mu.Unlock()
        time.Sleep(100 * time.Millisecond)
    }
}

// assignTaskToWorker sends a task to a worker and handles the response
func (c *CoordinatorService) assignTaskToWorker(task *types.Task, worker *types.WorkerStatus) {
    // Create TLS config for worker connection
    config := &tls.Config{
        InsecureSkipVerify: true, // For self-signed certificates
    }

    // Connect to worker using TLS
    conn, err := tls.Dial("tcp", worker.ID, config)
    if err != nil {
        log.Printf("Failed to connect to worker %s: %v\n", worker.ID, err)
        c.handleWorkerFailure(task, worker)
        return
    }
    
    client := rpc.NewClient(conn)
    defer client.Close()

    var resp types.ComputeResponse
    err = client.Call("Worker.ExecuteTask", task, &resp)
    if err != nil {
        log.Printf("Error executing task on worker %s: %v\n", worker.ID, err)
        c.handleWorkerFailure(task, worker)
        return
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    if task, exists := c.tasks[resp.TaskID]; exists {
        task.Result = resp.Result
        task.Completed = resp.Success
        task.Error = resp.Error
        delete(c.taskAssignments, task.ID)
    }
}

// handleWorkerFailure handles worker failures and task reassignment
func (c *CoordinatorService) handleWorkerFailure(task *types.Task, worker *types.WorkerStatus) {
    c.mu.Lock()
    defer c.mu.Unlock()

    delete(c.taskAssignments, task.ID)
    if c.retryCount[task.ID] < maxRetries {
        c.taskQueue = append(c.taskQueue, task.ID)
        c.retryCount[task.ID]++
        log.Printf("Reassigning task %s (attempt %d/%d)", task.ID, c.retryCount[task.ID], maxRetries)
    } else {
        log.Printf("Task %s failed after %d attempts", task.ID, maxRetries)
        task.Error = "maximum retry attempts exceeded"
        task.Completed = true
    }
}

func main() {
    useTLS := flag.Bool("tls", false, "Use TLS for secure communication")
    flag.Parse()

    coordinator := NewCoordinator()

    // Register RPC service
    server := rpc.NewServer()
    err := server.RegisterName("Coordinator", coordinator)
    if err != nil {
        log.Fatal("Failed to register RPC server:", err)
    }

    var listener net.Listener
    if *useTLS {
        // Generate TLS certificate
        cert, err := tlsutil.GenerateCertificate("localhost")
        if err != nil {
            log.Fatal("Failed to generate TLS certificate:", err)
        }

        // Create TLS configuration
        config := &tls.Config{
            Certificates: []tls.Certificate{cert},
            InsecureSkipVerify: true, // For self-signed certificates
        }

        // Start TLS listener
        listener, err = tls.Listen("tcp", ":1234", config)
        if err != nil {
            log.Fatal("Failed to start TLS listener:", err)
        }
        log.Println("Coordinator started with TLS on :1234")
    } else {
        listener, err = net.Listen("tcp", ":1234")
        if err != nil {
            log.Fatal("Failed to start listener:", err)
        }
        log.Println("Coordinator started on :1234")
    }

    // Accept connections
    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Println("Accept error:", err)
            continue
        }
        go server.ServeConn(conn)
    }
}