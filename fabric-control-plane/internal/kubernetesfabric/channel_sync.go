package kubernetesfabric

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/protobuf/proto"
)

// A successful genesis join does not mean existing definitions or transactions
// have been delivered. Require every peer's authenticated ledger observation to
// converge before querying chaincode. Query/trust errors are never suppressed.
func awaitPeerConvergence(ctx context.Context, interval time.Duration, peers []string, query func(string) (*common.BlockchainInfo, error)) error {
	if len(peers) == 0 {
		return secureDenied
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var target *common.BlockchainInfo
		converged := true
		for _, peer := range peers {
			info, e := query(peer)
			if e != nil {
				return e
			}
			if info == nil || info.Height == 0 || len(info.CurrentBlockHash) != 32 {
				return secureDenied
			}
			if target == nil {
				target = info
			} else if info.Height != target.Height || !bytes.Equal(info.CurrentBlockHash, target.CurrentBlockHash) {
				converged = false
			}
		}
		if converged {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("peer ledgers did not converge before chaincode verification: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (w *Worker) awaitSecureChannelPeers() error {
	peers := []string{}
	owners := map[string]string{}
	for _, node := range w.Spec.Spec.Nodes {
		if node.Role == "peer" {
			peers = append(peers, node.ID)
			owners[node.ID] = node.OrganizationRef
		}
	}
	ctx, cancel := context.WithTimeout(w.ctx, 90*time.Second)
	defer cancel()
	if w.argumentSamples == nil {
		w.argumentSamples = map[string]int{}
	}
	scoped := *w
	scoped.ctx = ctx
	return awaitPeerConvergence(ctx, time.Second, peers, func(peer string) (*common.BlockchainInfo, error) {
		raw, e := scoped.secureQueryAt(owners[peer], peer, "smpchannel", "qscc", "GetChainInfo", "smpchannel")
		if e != nil {
			return nil, e
		}
		var info common.BlockchainInfo
		if proto.Unmarshal(raw, &info) != nil {
			return nil, secureDenied
		}
		return &info, nil
	})
}
