// cmd/coordinator/main.go

package main

import (
	"log"
	"matrix-compute/pkg/types"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// Coordinator handles task distribution and worker management
type Coordinator struct {
	tasks     map[string]*types.Task
	workers   map[string]*types.WorkerStatus
	taskQueue []string // Queue of task IDs
	mu        sync.Mutex
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator() *Coordinator {
	c := &Coordinator{
		tasks:     make(map[string]*types.Task),
		workers:   make(map[string]*types.WorkerStatus),
		taskQueue: make([]string, 0),
	}
	go c.processTaskQueue()
	return c
}

// RegisterWorker registers a new worker with the coordinator
func (c *Coordinator) RegisterWorker(status *types.WorkerStatus, reply *bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.workers[status.ID] = status
	log.Printf("Worker %s registered\n", status.ID)
	*reply = true
	return nil
}

// SubmitTask handles new task submissions from clients
func (c *Coordinator) SubmitTask(req *types.ComputeRequest, resp *types.ComputeResponse) error {
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
func (c *Coordinator) GetTaskStatus(taskID string, resp *types.ComputeResponse) error {
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

// processTaskQueue continuously processes tasks in the queue
func (c *Coordinator) processTaskQueue() {
	for {
		c.mu.Lock()
		if len(c.taskQueue) > 0 && len(c.workers) > 0 {
			taskID := c.taskQueue[0]
			task := c.tasks[taskID]
			worker := c.getLeastBusyWorker()

			if worker != nil {
				// Remove task from queue
				c.taskQueue = c.taskQueue[1:]

				// Assign task to worker
				go c.assignTaskToWorker(task, worker)
			}
		}
		c.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

// getLeastBusyWorker returns the worker with the lowest task count
func (c *Coordinator) getLeastBusyWorker() *types.WorkerStatus {
	var leastBusy *types.WorkerStatus
	minTasks := -1

	for _, worker := range c.workers {
		if worker.Available && (minTasks == -1 || worker.TaskCount < minTasks) {
			leastBusy = worker
			minTasks = worker.TaskCount
		}
	}

	return leastBusy
}

// assignTaskToWorker sends a task to a worker and handles the response
func (c *Coordinator) assignTaskToWorker(task *types.Task, worker *types.WorkerStatus) {
	client, err := rpc.Dial("tcp", worker.ID)
	if err != nil {
		log.Printf("Failed to connect to worker %s: %v\n", worker.ID, err)
		c.mu.Lock()
		c.taskQueue = append(c.taskQueue, task.ID) // Re-queue the task
		c.mu.Unlock()
		return
	}
	defer client.Close()

	var resp types.ComputeResponse
	err = client.Call("Worker.ExecuteTask", task, &resp)
	if err != nil {
		log.Printf("Error executing task on worker %s: %v\n", worker.ID, err)
		c.mu.Lock()
		c.taskQueue = append(c.taskQueue, task.ID) // Re-queue the task
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	if task, exists := c.tasks[resp.TaskID]; exists {
		task.Result = resp.Result
		task.Completed = resp.Success
		task.Error = resp.Error
	}
	c.mu.Unlock()
}

func main() {
	coordinator := NewCoordinator()

	// Register RPC service
	server := rpc.NewServer()
	err := server.Register(coordinator)
	if err != nil {
		log.Fatal("Failed to register RPC server:", err)
	}

	// Start listening for connections
	listener, err := net.Listen("tcp", ":1234")
	if err != nil {
		log.Fatal("Failed to start listener:", err)
	}

	log.Println("Coordinator started on :1234")
	
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