// pkg/matrix/matrix.go

package matrix

import (
	"errors"
	"fmt"
)

// Matrix represents a 2D matrix
type Matrix struct {
	Rows    int
	Cols    int
	Data    [][]float64
}

// NewMatrix creates a new matrix with the specified dimensions
func NewMatrix(rows, cols int) (*Matrix, error) {
	if rows <= 0 || cols <= 0 {
		return nil, errors.New("matrix dimensions must be positive")
	}

	// Initialize the matrix with zeros
	data := make([][]float64, rows)
	for i := range data {
		data[i] = make([]float64, cols)
	}

	return &Matrix{
		Rows: rows,
		Cols: cols,
		Data: data,
	}, nil
}

// Set sets the value at the specified position
func (m *Matrix) Set(row, col int, value float64) error {
	if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
		return errors.New("index out of bounds")
	}
	m.Data[row][col] = value
	return nil
}

// Get retrieves the value at the specified position
func (m *Matrix) Get(row, col int) (float64, error) {
	if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
		return 0, errors.New("index out of bounds")
	}
	return m.Data[row][col], nil
}

// String returns a string representation of the matrix
func (m *Matrix) String() string {
	result := fmt.Sprintf("Matrix (%dx%d):\n", m.Rows, m.Cols)
	for i := 0; i < m.Rows; i++ {
		result += "["
		for j := 0; j < m.Cols; j++ {
			result += fmt.Sprintf(" %6.2f", m.Data[i][j])
		}
		result += " ]\n"
	}
	return result
}