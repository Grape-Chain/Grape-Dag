package postgres

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/db/base"
	golog "github.com/ipfs/go-log/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresManager struct {
	pool *pgxpool.Pool
}

var logger golog.EventLogger

func init() {
	logger = golog.Logger("p2p-db:postgres")
}

func (m *PostgresManager) Connect() error {
	if m == nil {
		return errors.New("PostgresDbMngr instance is nil")
	}
	ctxPool, cancelPool := context.WithTimeout(context.Background(), config.DB_CTX_TIMEOUT*time.Second)
	defer cancelPool()
	p, err := pgxpool.New(ctxPool, os.Getenv("POSTGRES_URL"))
	if err != nil {
		logger.Errorf("Failed to create pgx pool: %v", err)
	}

	ctxPing, cancelPing := context.WithTimeout(context.Background(), config.DB_CTX_TIMEOUT*time.Second)
	defer cancelPing()
	err = m.pool.Ping(ctxPing)
	if err != nil {
		p.Close()
		logger.Errorf("Failed to ping database: %v", err)
		return err
	}
	m.pool = p
	logger.Infof("Db connection successfully established")
	return nil
}

func (m *PostgresManager) Disconnect() error {
	if m == nil {
		return errors.New("PostgresDbMngr instance is nil")
	}
	if m.pool != nil {
		m.pool.Close()
		m.pool = nil
	}
	return nil
}

func (m *PostgresManager) Write(stats *base.TxStats) error {
	return errors.New("Stats Writer not implemented")
}

func (m *PostgresManager) WriteMany(stats []interface{}) error {
	return errors.New("Stats WriterMany not implemented")
}
