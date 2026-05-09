package db

import (
	"github.com/VG-Grape/luna/db/base"
	"github.com/VG-Grape/luna/db/mongo"
	"github.com/VG-Grape/luna/db/postgres"
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
