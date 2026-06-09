package txgen

import (
	"context"
	"fmt"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/enescakir/emoji"
)

type WatchDogCommand struct {
	timeout time.Duration
	retries int32
}
type WatchDogResponse struct {
	Status pb.WatchDogResponse_PEER_STATUS
}

func (c *WatchDogCommand) AdviseColor() string {
	return "fgHiMagenta"
}
func (c *WatchDogCommand) Execute(cltService *pb.RoboTraderClient) any {
	var err error = nil
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout+time.Second*10)
	defer cancel()
	response, err := (*cltService).WatchDog(ctx, &pb.WatchDogRequest{Timeout: int64(c.timeout), Retries: c.retries})
	if err != nil {
		fmt.Printf("Failed to obtain WatchDog response %s\n", err.Error())
		fmt.Println(">>> FAILURE <<<")
		return nil
	}
	return &WatchDogResponse{Status: response.Status}
}
func (c *WatchDogCommand) ProcessResult(i any) (CommandStatus, error) {
	response, ok := i.(*WatchDogResponse)
	if !ok {
		return CS_ER, fmt.Errorf("failed to cast result to WatchDogResponse. %t", ok)
	}
	fmt.Printf("\n%s  ~ Received status %d\n", emoji.SpeechBalloon, response.Status)
	if response.Status == pb.WatchDogResponse_RUNNING {
		fmt.Printf("%s  ~ STATUS: RUNNING\n", emoji.CheckMarkButton)
		return CS_OK, nil
	} else {
		fmt.Printf("%s  ~ STATUS: KILLED\n", emoji.Collision)
		return CS_ER, nil
	}
}
func (c *WatchDogCommand) Init(args *CommandArgs) error {
	c.timeout = time.Second * time.Duration(args.Timeout)
	c.retries = int32(args.Retries)
	return nil
}
