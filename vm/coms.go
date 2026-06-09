package vm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	golog "github.com/ipfs/go-log/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const default_port int = 29299
const host string = "localhost"

var clogger golog.EventLogger = golog.Logger("vm-client")
var cachedVmClient pb.SCVmServiceClient
var vmCtx VmExecContext

type contextEnhancedSCVmServiceClient struct {
	pb.SCVmServiceClient
}

type VmExecContext struct {
	execLock sync.RWMutex
	execTx   *tx.Txv1
}

func CurrentExecutingTx() *tx.Txv1 {
	vmCtx.execLock.RLock()
	defer vmCtx.execLock.RUnlock()
	return vmCtx.execTx
}

func setTx(tx *tx.Txv1) {
	vmCtx.execLock.Lock()
	defer vmCtx.execLock.Unlock()
	vmCtx.execTx = tx
}

func (c *contextEnhancedSCVmServiceClient) RunCall(ctx context.Context, in *pb.WriteContractRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	t := &tx.Txv1{}
	t.UnmarshalBinary(in.Tx)
	setTx(t)
	defer func() {
		setTx(nil)
	}()
	return c.SCVmServiceClient.RunCall(ctx, in, opts...)
}

func ConnectToVm() (pb.SCVmServiceClient, error) {
	if cachedVmClient != nil {
		return cachedVmClient, nil
	}
	port := default_port
	if config.GetConfig().Peer.VmServerPort == 0 {
		clogger.Warnf("Using default VM Server's port: %d", default_port)
	} else {
		port = config.GetConfig().Peer.VmServerPort
	}
	vmGrpcURI := fmt.Sprintf("%s:%d", host, port)
	clogger.Infof("Connecting to Smart Contract VM via gRPC by URI: %s", vmGrpcURI)

	conn, err := grpc.Dial(vmGrpcURI, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	go func() {
		utils.WaitOnSignal([]os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL})
		conn.Close()
	}()
	if err != nil {
		return nil, fmt.Errorf("error connecting VM via gRPC %v+", err)
	}
	clogger.Infof("VM is connected by URI %s", vmGrpcURI)
	createdVmClient := pb.NewSCVmServiceClient(conn)
	cachedVmClient = &contextEnhancedSCVmServiceClient{createdVmClient}
	return cachedVmClient, nil
}
