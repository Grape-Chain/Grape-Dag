package mongo

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/VG-Grape/luna/db/base"
	golog "github.com/ipfs/go-log/v2"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type MongoManager struct {
	dbclient *mongo.Client
	dbcoll   *mongo.Collection
}

const (
	STATS_COLLECTION     = "txstats"
	MONGODB_URI_TEMPLATE = "mongodb://%s:%s@%s:%s/?maxPoolSize=20&w=majority"
)

var logger golog.EventLogger

func init() {
	logger = golog.Logger("p2p-db")
}

func (m *MongoManager) Connect() error {
	// We assume two separate criteria are to be present to enable stats collection
	// and writing to mongodb: 1. stats option to enable stats 2. env vars to point
	// to a working/accessable mongodb instance
	var (
		username, passwd, database, db_ip, db_port string
		flag                                       bool
	)

	if username, flag = os.LookupEnv("MONGO_INITDB_ROOT_USERNAME"); !flag {
		logger.Errorf("Env var %s not set", "MONGO_INITDB_ROOT_USERNAME")
		return nil
	}
	if passwd, flag = os.LookupEnv("MONGO_INITDB_ROOT_PASSWORD"); !flag {
		logger.Errorf("Env var %s not set", "MONGO_INITDB_ROOT_PASSWORD")
		return nil
	}
	if database, flag = os.LookupEnv("MONGO_INITDB_DATABASE"); !flag {
		logger.Errorf("Env var %s not set", "MONGO_INITDB_DATABASE")
		return nil
	}
	if db_ip, flag = os.LookupEnv("MONGO_INITDB_IP"); !flag {
		logger.Errorf("Env var %s not set", "MONGO_INITDB_IP")
		return nil
	}
	if db_port, flag = os.LookupEnv("MONGO_INITDB_PORT"); !flag {
		logger.Errorf("Env var %s not set", "MONGO_INITDB_PORT")
		return nil
	}

	MONGODB_URI := fmt.Sprintf(MONGODB_URI_TEMPLATE, username, passwd, db_ip, db_port)

	client, err := mongo.Connect(
		context.TODO(),
		options.Client().ApplyURI(MONGODB_URI),
	)
	if err != nil {
		logger.Errorf("Connect to %s failed. %s", MONGODB_URI, err)
		return err
	}
	// If the connection is established, wait for ping for no more than 5 sec
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		logger.Errorf("Failed to ping %s. %v", MONGODB_URI, err)
		return err
	}

	coll := client.Database(database).Collection(STATS_COLLECTION)
	logger.Info("[SUCCESS] Connected and pinged the db")
	m.dbclient, m.dbcoll = client, coll
	return nil
}

func (m *MongoManager) Disconnect() error {
	if m != nil && m.dbclient != nil {
		if err := m.dbclient.Disconnect(context.Background()); err != nil {
			logger.Errorf("Failed to disconnect from db. %v", err)
			return err
		}
	}
	return nil
}

func (dbmngr *MongoManager) Write(stats *base.TxStats) error {
	if stats != nil && dbmngr != nil && dbmngr.dbclient != nil && dbmngr.dbcoll != nil {
		// stats.Timestamp = primitive.Timestamp{T: uint32(time.Now().Unix())}
		_, err := dbmngr.dbcoll.InsertOne(context.Background(), *stats)
		// logger.Infof("Write Transaction:\n%s", stats)
		if err != nil {
			logger.Errorf("Insert Tx:[%t]%s failed. %s", stats.TxType, stats.TxID, err.Error())
			return err
		}
	}
	return nil
}

func (dbmngr *MongoManager) WriteMany(stats []interface{}) error {
	if stats != nil && dbmngr != nil && dbmngr.dbclient != nil && dbmngr.dbcoll != nil {
		// stats.Timestamp = primitive.Timestamp{T: uint32(time.Now().Unix())}
		_, err := dbmngr.dbcoll.InsertMany(context.Background(), stats)
		// logger.Infof("Write Transaction:\n%s", stats)
		if err != nil {
			logger.Errorf("InsertMany failed. %s", err.Error())
			return err
		}
	}
	return nil
}
