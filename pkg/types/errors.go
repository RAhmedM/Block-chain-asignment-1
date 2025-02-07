// pkg/types/errors.go

package types

// MatrixError represents an error in matrix operations
type MatrixError string

func (e MatrixError) Error() string {
	return string(e)
}