package base

import (
	"fmt"
)

// Publisher: TxType false, Subscriber: TxType true
type TxStats struct {
	PeerID      string `bson:"peer,omitempty"`
	TxID        string `bson:"id,omitempty"`
	TxSender    string `bson:"sender,omitempty"`
	TxRecipient string `bson:"recipient,omitempty"`
	TxAmount    string `bson:"amount,omitempty"`
	TxType      string `bson:"source,omitemtpy"`
	TxTime      string `bson:"txtime"`
	RcTime      string `bson:"rctime"`
	DbTime      string `bson:"dbtime"`
	DiffTime    string `bson:"diff"`
	MsgQueue    int64  `bson:"queue_size"`
	// Timestamp   primitive.Timestamp `bson:"timestamp,omitempty"`
}

func (txs *TxStats) String() string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		txs.PeerID, txs.TxSender, txs.TxRecipient, txs.TxAmount, txs.TxType, txs.TxTime, txs.RcTime, txs.DbTime)
	//time.Unix(int64(txs.Timestamp.T), 0).String())
}
