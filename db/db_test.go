package db

import (
	"testing"
)

func TestDb(t *testing.T) {
	m := Create("postgres")
	if m == nil {
		t.Fatalf("Failed to create ")
	}
}
