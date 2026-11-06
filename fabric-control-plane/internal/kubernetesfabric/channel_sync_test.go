package kubernetesfabric

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

func TestJoinedPeerMustCatchUpBeforeChaincodeVerification(t *testing.T) {
	rounds := 0
	e := awaitPeerConvergence(context.Background(), time.Millisecond, []string{"existing", "joined"}, func(peer string) (*common.BlockchainInfo, error) {
		if peer == "existing" {
			rounds++
		}
		height := uint64(7)
		hash := byte(7)
		if peer == "joined" && rounds == 1 {
			height, hash = 1, 1 // Genesis is usable, but has no chaincode definition.
		}
		return &common.BlockchainInfo{Height: height, CurrentBlockHash: bytes.Repeat([]byte{hash}, 32)}, nil
	})
	if e != nil || rounds != 2 {
		t.Fatal("genesis-only peer did not wait for committed history", rounds, e)
	}
}

func TestChannelConvergenceRejectsTrustErrorsAndForks(t *testing.T) {
	for _, reason := range []string{"tls-trust", "authorization", "msp-signature"} {
		calls := 0
		denied := errors.New(reason)
		e := awaitPeerConvergence(context.Background(), time.Millisecond, []string{"peer"}, func(string) (*common.BlockchainInfo, error) {
			calls++
			return nil, denied
		})
		if !errors.Is(e, denied) || calls != 1 {
			t.Fatal("trust/authorization error was retried or ignored", reason)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	e := awaitPeerConvergence(ctx, time.Millisecond, []string{"a", "b"}, func(peer string) (*common.BlockchainInfo, error) {
		return &common.BlockchainInfo{Height: 7, CurrentBlockHash: bytes.Repeat([]byte(peer), 32)}, nil
	})
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("equal heights on different ledgers were accepted", e)
	}
}
