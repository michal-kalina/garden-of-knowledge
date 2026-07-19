package database

import (
	"strconv"
	"strings"
)

// VectorLiteral renders a float32 slice in pgvector's text input format:
// [0.1,0.2,...]. Going through the text representation keeps the project on
// plain database/sql without a pgvector driver binding. Used by the worker
// (chunk inserts) and retrieval (query vector).
func VectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
