# BlockChain Assignment 1

## Overview
The Matrix Compute System is a distributed computing platform designed to perform matrix operations across multiple worker nodes. It implements a coordinator-worker architecture where tasks are distributed and processed in parallel, providing scalability and fault tolerance.

## System Architecture

### Components
1. **Coordinator**
   - Acts as the central task manager
   - Handles client requests
   - Manages worker registration and health monitoring
   - Implements task distribution and load balancing
   - Located in `cmd/coordinator/main.go`

2. **Workers**
   - Execute matrix operations
   - Report health status to coordinator
   - Support multiple concurrent operations
   - Located in `cmd/worker/main.go`

3. **Client**
   - Submits computation requests
   - Monitors task progress
   - Located in `cmd/client/main.go`

### Communication
- Uses Go's RPC (Remote Procedure Call) framework
- Optional TLS support for secure communication
- Health check system with configurable timeouts
- Exponential backoff for retries

## Supported Operations

The system supports the following matrix operations:
1. **Addition** (`types.Addition`)
   - Adds two matrices of the same dimensions
   - Returns a matrix where each element is the sum of corresponding elements

2. **Transpose** (`types.Transpose`)
   - Transposes a single matrix
   - Converts rows to columns and vice versa

3. **Multiplication** (`types.Multiplication`)
   - Multiplies two matrices where the first matrix's columns match the second matrix's rows
   - Returns the product matrix

## Core Features

### Fault Tolerance
- Worker health monitoring
- Automatic task reassignment on worker failure
- Configurable retry attempts (default: 3)
- Dead worker detection and cleanup

### Load Balancing
- Tasks distributed to least busy workers
- Worker load tracked through task count
- Dynamic worker registration and deregistration

### Security
- Optional TLS encryption
- Self-signed certificate generation
- Secure communication between all components

## Usage

### Starting the Coordinator
```bash
go run cmd/coordinator/main.go [flags]

Flags:
  -tls    Enable TLS encryption (default: false)
  -v      Enable verbose logging (default: false)
```

### Starting a Worker
```bash
go run cmd/worker/main.go [flags]

Flags:
  -port int              Port to listen on (default: random)
  -coordinator string    Coordinator address (default: "localhost:1234")
  -tls                   Use TLS for secure communication (default: false)
```

### Running the Client
```bash
go run cmd/client/main.go [flags]

Flags:
  -coordinator-host string   Coordinator hostname or IP (default: "localhost")
  -coordinator-port int      Coordinator port (default: 1234)
  -op string                Operation to perform: multiply, add, transpose (default: "multiply")
  -rows int                 Number of rows in matrices (default: 3)
  -cols int                 Number of columns in matrices (default: 3)
  -tls                      Use TLS for secure communication (default: false)
  -retry int                Number of connection retry attempts (default: 3)
```

## Configuration Constants

### Coordinator Settings
```go
const (
    workerTimeout     = 10 * time.Second
    healthCheckPeriod = 5 * time.Second
    maxRetries        = 3
)
```

## Error Handling

The system implements comprehensive error handling for various scenarios:

1. **Matrix Operation Errors**
   - Invalid dimensions
   - Nil matrices
   - Index out of bounds

2. **Network Errors**
   - Connection failures
   - Timeout errors
   - Worker unavailability

3. **Task Processing Errors**
   - Maximum retry attempts exceeded
   - Worker failures during computation
   - Invalid operation types

## Testing

The system includes extensive test coverage:

1. **Unit Tests**
   - Matrix operations
   - Worker functionality
   - Error handling

2. **Integration Tests**
   - Distributed computation
   - Worker coordination
   - Fault tolerance

3. **Edge Cases**
   - Zero-size matrices
   - Nil matrices
   - Invalid dimensions

### Running Tests
```bash
go test ./... -v
```

## Package Structure

```
matrix-compute/
├── cmd/
│   ├── coordinator/
│   │   └── main.go
│   ├── worker/
│   │   ├── main.go
│   │   └── worker_test.go
│   └── client/
│       └── main.go
├── pkg/
│   ├── matrix/
│   │   ├── matrix.go
│   │   └── matrix_test.go
│   ├── types/
│   │   ├── errors.go
│   │   └── types.go
│   ├── rpc/
│   │   └── rpc.go
│   └── tls/
│       └── cert.go
```

## Performance Considerations

1. **Task Distribution**
   - Dynamic worker allocation
   - Load-based task assignment
   - Concurrent task processing

2. **Resource Management**
   - Worker health monitoring
   - Task queue management
   - Connection pooling

3. **Scalability**
   - Horizontal scaling through worker addition
   - Dynamic worker registration
   - Load balancing across workers

## Best Practices

1. **Error Handling**
   - Always check return errors
   - Implement proper cleanup in defer statements
   - Use context for timeouts

2. **Security**
   - Enable TLS in production
   - Use proper certificate management
   - Implement proper access controls

3. **Monitoring**
   - Enable verbose logging when needed
   - Monitor worker health
   - Track task completion rates

## Limitations and Future Improvements

1. **Current Limitations**
   - Single coordinator (potential bottleneck)
   - In-memory task queue
   - Simple load balancing algorithm

2. **Potential Improvements**
   - Distributed coordinator architecture
   - Persistent task queue
   - Advanced load balancing strategies
   - Better metrics and monitoring
   - Task priority system
   - Worker specialization

## Troubleshooting

Common issues and solutions:

1. **Connection Issues**
   - Verify coordinator address and port
   - Check network connectivity
   - Ensure TLS settings match between components

2. **Task Failures**
   - Check worker logs for errors
   - Verify matrix dimensions
   - Ensure sufficient system resources

3. **Performance Issues**
   - Monitor worker load
   - Check network latency
   - Adjust timeout settings if needed