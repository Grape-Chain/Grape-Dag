package base

type DbManagerBase interface {
	Connect() error
	Disconnect() error
}

type DbFactory interface {
	Create() DbManager
}

type DbFactories struct {
	Factories map[string]DbFactory
}

type StatsWriter interface {
	Write(stats *TxStats) error
	WriteMany(stats []interface{}) error
}

type DbManager interface {
	DbManagerBase
	StatsWriter
}
