package postgres

import "github.com/Grape-Chain/Grape-Dag/db/base"

type PostgresFactory struct{}

func (f *PostgresFactory) Create() base.DbManager {
	m := &PostgresManager{}
	if err := m.Connect(); err != nil {
		return nil
	}
	return m
}
