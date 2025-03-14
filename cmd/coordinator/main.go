// cmd/coordinator/main.go

package main

import (
    "crypto/tls"
    "flag"
    "log"
    "matrix-compute/pkg/types"
    tlsutil "matrix-compute/pkg/tls"
    "fmt"
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
    verbose        bool
    tlsCert        tls.Certificate    // Add this field
}


// NewCoordinator creates a new coordinator instance
func NewCoordinator(verbose bool, cert tls.Certificate) *CoordinatorService {
    c := &CoordinatorService{
        tasks:           make(map[string]*types.Task),
        workers:         make(map[string]*types.WorkerStatus),
        taskQueue:       make([]string, 0),
        retryCount:      make(map[string]int),
        taskAssignments: make(map[string]string),
        verbose:         verbose,
        tlsCert:        cert,           // Initialize the certificate
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
    log.Printf("[INFO] Worker %s registered successfully\n", status.ID)
    if c.verbose {
        log.Printf("[DEBUG] Total workers registered: %d\n", len(c.workers))
        for id := range c.workers {
            log.Printf("[DEBUG] - Worker %s is registered\n", id)
        }
    }
    *reply = true
    return nil
}

// SubmitTask handles new task submissions from clients
func (c *CoordinatorService) SubmitTask(req *types.ComputeRequest, resp *types.ComputeResponse) error {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.verbose {
        log.Printf("[DEBUG] Received task submission: %s\n", req.TaskID)
        log.Printf("[DEBUG] Number of available workers: %d\n", len(c.workers))
    }

    // Verify we have workers available
    if len(c.workers) == 0 {
        log.Printf("[ERROR] No workers registered to handle task %s\n", req.TaskID)
        resp.Error = "no workers available"
        resp.Success = false
        return nil
    }

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

    if c.verbose {
        log.Printf("[DEBUG] Task %s added to queue. Queue length: %d\n", req.TaskID, len(c.taskQueue))
    }

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

    if c.verbose {
        log.Printf("[DEBUG] Status request for task %s: completed=%v, error=%s\n", 
            taskID, task.Completed, task.Error)
    }

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
        if c.verbose {
            log.Printf("[DEBUG] Updated status for worker %s: available=%v, taskCount=%d\n",
                status.ID, status.Available, status.TaskCount)
        }
    } else {
        log.Printf("[WARN] Received status update from unregistered worker: %s\n", status.ID)
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
                log.Printf("[WARN] Worker %s appears to be dead, removing...\n", id)
                delete(c.workers, id)

                // Reassign tasks from failed worker
                for taskID, workerID := range c.taskAssignments {
                    if workerID == id {
                        if c.retryCount[taskID] < maxRetries {
                            c.taskQueue = append(c.taskQueue, taskID)
                            c.retryCount[taskID]++
                            delete(c.taskAssignments, taskID)
                            log.Printf("[INFO] Reassigning task %s (attempt %d/%d)\n", 
                                taskID, c.retryCount[taskID], maxRetries)
                        } else {
                            log.Printf("[ERROR] Task %s failed after %d attempts\n", taskID, maxRetries)
                            if task, exists := c.tasks[taskID]; exists {
                                task.Error = "maximum retry attempts exceeded"
                                task.Completed = true
                            }
                        }
                    }
                }
            }
        }

        if c.verbose {
            log.Printf("[DEBUG] Health check complete. Active workers: %d\n", len(c.workers))
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

        // Calculate load score based on task count
        loadScore := float64(worker.TaskCount)
        if selected == nil || loadScore < minLoad {
            selected = worker
            minLoad = loadScore
        }
    }

    if c.verbose && selected != nil {
        log.Printf("[DEBUG] Selected worker %s (load: %.2f) for next task\n", 
            selected.ID, minLoad)
    }

    return selected
}

// processTaskQueue continuously processes tasks in the queue
func (c *CoordinatorService) processTaskQueue() {
    for {
        c.mu.Lock()
        if len(c.taskQueue) > 0 {
            if c.verbose {
                log.Printf("[DEBUG] Processing task queue. Length: %d\n", len(c.taskQueue))
            }

            taskID := c.taskQueue[0]
            if worker := c.getLeastBusyWorker(); worker != nil {
                task := c.tasks[taskID]
                c.taskQueue = c.taskQueue[1:]
                c.taskAssignments[taskID] = worker.ID
                
                log.Printf("[INFO] Assigning task %s to worker %s\n", taskID, worker.ID)
                
                go c.assignTaskToWorker(task, worker)
            } else {
                log.Printf("[WARN] No available workers to process task %s\n", taskID)
            }
        }
        c.mu.Unlock()
        time.Sleep(100 * time.Millisecond)
    }
}


// assignTaskToWorker sends a task to a worker and handles the response
func (c *CoordinatorService) assignTaskToWorker(task *types.Task, worker *types.WorkerStatus) {
    if c.verbose {
        log.Printf("[DEBUG] Attempting to connect to worker %s for task %s\n", 
            worker.ID, task.ID)
    }

    var client *rpc.Client
    var err error

    // Try to connect to the worker
    for attempt := 1; attempt <= maxRetries; attempt++ {
        if c.verbose {
            log.Printf("[DEBUG] Connection attempt %d to worker %s\n", attempt, worker.ID)
        }

        // Create TLS configuration for client
        config := &tls.Config{
            InsecureSkipVerify: true,
            Certificates:       []tls.Certificate{c.tlsCert}, // Add this field to CoordinatorService
        }

        // Connect with TLS
        conn, dialErr := tls.Dial("tcp", worker.ID, config)
        if dialErr == nil {
            client = rpc.NewClient(conn)
            break
        }
        err = dialErr

        if attempt == maxRetries {
            log.Printf("[ERROR] Failed to connect to worker %s after %d attempts: %v\n", 
                worker.ID, maxRetries, err)
            c.handleWorkerFailure(task, worker)
            return
        }

        time.Sleep(time.Second)
    }
    defer client.Close()

    var resp types.ComputeResponse
    err = client.Call("Worker.ExecuteTask", task, &resp)
    if err != nil {
        log.Printf("[ERROR] Error executing task on worker %s: %v\n", worker.ID, err)
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
        
        if c.verbose {
            log.Printf("[DEBUG] Task %s completed by worker %s: success=%v, error=%s\n",
                task.ID, worker.ID, resp.Success, resp.Error)
        }
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
        log.Printf("[INFO] Reassigning task %s (attempt %d/%d)\n", 
            task.ID, c.retryCount[task.ID], maxRetries)
    } else {
        log.Printf("[ERROR] Task %s failed after %d attempts\n", task.ID, maxRetries)
        task.Error = "maximum retry attempts exceeded"
        task.Completed = true
    }
}

func main() {
    useTLS := flag.Bool("tls", false, "Use TLS for secure communication")
    verbose := flag.Bool("v", false, "Enable verbose logging")
    host := flag.String("host", "0.0.0.0", "Host address to listen on")
    flag.Parse()

    var cert tls.Certificate
    var err error

    if *useTLS {
        // Generate TLS certificate
        cert, err = tlsutil.GenerateCertificate(*host) // Use the host parameter
        if err != nil {
            log.Fatal("[FATAL] Failed to generate TLS certificate:", err)
        }
    }

    coordinator := NewCoordinator(*verbose, cert)

    // Register RPC service
    server := rpc.NewServer()
    err = server.RegisterName("Coordinator", coordinator)
    if err != nil {
        log.Fatal("[FATAL] Failed to register RPC server:", err)
    }

    var listener net.Listener
    if *useTLS {
        // Create TLS configuration
        config := &tls.Config{
            Certificates: []tls.Certificate{cert},
            ClientAuth: tls.NoClientCert,
            MinVersion: tls.VersionTLS12,
            InsecureSkipVerify: true,
        }

        // Start TLS listener on specified host
        listener, err = tls.Listen("tcp", fmt.Sprintf("%s:1234", *host), config)
        if err != nil {
            log.Fatal("[FATAL] Failed to start TLS listener:", err)
        }
        log.Printf("[INFO] Coordinator started with TLS on %s:1234\n", *host)
    } else {
        listener, err = net.Listen("tcp", fmt.Sprintf("%s:1234", *host))
        if err != nil {
            log.Fatal("[FATAL] Failed to start listener:", err)
        }
        log.Printf("[INFO] Coordinator started on %s:1234\n", *host)
    }


    // Accept connections
    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Println("[ERROR] Accept error:", err)
            continue
        }

        if *useTLS {
            tlsConn := conn.(*tls.Conn)
            if err := tlsConn.Handshake(); err != nil {
                log.Printf("[ERROR] TLS handshake failed: %v\n", err)
                conn.Close()
                continue
            }
            if verbose != nil && *verbose {
                log.Printf("[DEBUG] TLS handshake successful with %s\n", tlsConn.RemoteAddr())
            }
        }

        go server.ServeConn(conn)
    }
}