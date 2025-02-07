// cmd/worker/worker_test.go

package main

import (
    "matrix-compute/pkg/matrix"
    "testing"
    "fmt"
)

func TestTranspose(t *testing.T) {
    // Create a small test matrix
    m, err := matrix.NewMatrix(3, 3)
    if err != nil {
        t.Fatalf("Failed to create matrix: %v", err)
    }

    // Set test values
    m.Set(0, 0, 1.0)
    m.Set(0, 1, 2.0)
    m.Set(0, 2, 3.0)
    m.Set(1, 0, 4.0)
    m.Set(1, 1, 5.0)
    m.Set(1, 2, 6.0)
    m.Set(2, 0, 7.0)
    m.Set(2, 1, 8.0)
    m.Set(2, 2, 9.0)

    fmt.Println("Original matrix:")
    fmt.Println(m)

    // Create worker and perform transpose
    w := &Worker{}
    result, err := w.Transpose(m)
    if err != nil {
        t.Fatalf("Transpose failed: %v", err)
    }

    fmt.Println("\nTransposed matrix:")
    fmt.Println(result)

    // Verify the transpose
    for i := 0; i < 3; i++ {
        for j := 0; j < 3; j++ {
            original, _ := m.Get(i, j)
            transposed, _ := result.Get(j, i)
            if original != transposed {
                t.Errorf("Mismatch at position (%d,%d): expected %f, got %f", i, j, original, transposed)
            }
        }
    }
}