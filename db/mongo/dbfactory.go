package mongo

import (
	"github.com/VG-Grape/luna/db/base"
)

type MongoFactory struct {
}

func (f *MongoFactory) Create() base.DbManager {
	m := &MongoManager{}
	if err := m.Connect(); err != nil {
		return nil
	}
	return m
}
