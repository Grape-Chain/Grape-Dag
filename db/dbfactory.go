package db

import (
	"github.com/Grape-Chain/Grape-Dag/db/base"
	"github.com/Grape-Chain/Grape-Dag/db/mongo"
	"github.com/Grape-Chain/Grape-Dag/db/postgres"
)

var factories *base.DbFactories

func init() {
	factories = initDbFactories()
}

func initDbFactories() *base.DbFactories {
	return &base.DbFactories{
		Factories: map[string]base.DbFactory{
			"postgres": &postgres.PostgresFactory{},
			"mongo":    &mongo.MongoFactory{},
		},
	}
}

func Create(db_type string) base.DbManager {
	val, ok := factories.Factories[db_type]
	if ok {
		return val.Create()
	}
	return nil
}
