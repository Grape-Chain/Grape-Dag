package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestConnect(t *testing.T) {
	db_url := os.Getenv("LUNA_TEST_POSTGRES_URI")
	if db_url == "" {
		t.Skip("LUNA_TEST_POSTGRES_URI not set; skipping")
	}
	ctxConnect, cancelConnect := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelConnect()
	conn, err := pgx.Connect(ctxConnect, db_url)
	if err != nil {
		t.Fatalf("Unable to connect to database: %v", err)
	}
	defer conn.Close(context.Background())
	ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	if err = conn.Ping(ctxPing); err != nil {
		t.Fatalf("Unable to ping database: %v", err)
	}
}
