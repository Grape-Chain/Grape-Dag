package db

import (
	"os"
	"testing"
)

func TestUnknownBackendIsRejected(t *testing.T) {
	if m := Create("nosuchdb"); m != nil {
		t.Fatalf("Create returned a manager for an unknown backend")
	}
}

// The factory connects eagerly and returns nil when it cannot, so this needs a
// reachable database. It used to panic instead: Connect pinged the pool field
// before assigning it, so the very first call dereferenced nil.
func TestPostgresFactory(t *testing.T) {
	if os.Getenv("POSTGRES_URL") == "" {
		t.Skip("POSTGRES_URL is not set; nothing to connect to")
	}
	if m := Create("postgres"); m == nil {
		t.Fatalf("Create(postgres) returned nil with POSTGRES_URL set")
	}
}

// Whatever the environment, asking for a backend must not take the process down.
func TestCreateDoesNotPanicWithoutADatabase(t *testing.T) {
	for _, backend := range []string{"postgres", "mongo"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Create(%s) panicked: %v", backend, r)
				}
			}()
			Create(backend)
		}()
	}
}
