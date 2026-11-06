package kubernetesfabric

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
)

type generatedConfig struct {
	Genesis []byte `json:"genesis"`
	Update  []byte `json:"update"`
	Changed bool   `json:"changed"`
}

func decodeChannelConfig(raw []byte, channel string) (*common.Config, *common.Block, error) {
	var block common.Block
	var env common.Envelope
	var payload common.Payload
	var header common.ChannelHeader
	var config common.ConfigEnvelope
	if proto.Unmarshal(raw, &block) != nil || block.Header == nil || block.Data == nil || len(block.Data.Data) != 1 {
		return nil, nil, secureDenied
	}
	sum := sha256.Sum256(bytes.Join(block.Data.Data, nil))
	if !bytes.Equal(sum[:], block.Header.DataHash) {
		return nil, nil, secureDenied
	}
	if proto.Unmarshal(block.Data.Data[0], &env) != nil || proto.Unmarshal(env.Payload, &payload) != nil || payload.Header == nil || proto.Unmarshal(payload.Header.ChannelHeader, &header) != nil || header.ChannelId != channel || header.Type != int32(common.HeaderType_CONFIG) || proto.Unmarshal(payload.Data, &config) != nil || config.Config == nil || config.Config.ChannelGroup == nil {
		return nil, nil, secureDenied
	}
	return config.Config, &block, nil
}

func (w *Worker) dynamicConfig(c domain.Channel, current []byte) (generatedConfig, error) {
	input := Object{"namespace": w.Namespace, "channel": c.ID, "current": current, "organizations": []any{}, "applicationMSPs": []string{}, "orderers": []any{}}
	for _, o := range w.Spec.Spec.Organizations {
		sign, e := w.get("fabriccas", w.caName(w.authority(o.ID, "ca")))
		if e != nil {
			return generatedConfig{}, e
		}
		tlsCA, e := w.get("fabriccas", w.caName(w.authority(o.ID, "tlsca")))
		if e != nil {
			return generatedConfig{}, e
		}
		anchors := []string{}
		for _, p := range w.channelNodes(c, "peer") {
			for _, a := range w.Spec.Spec.Configuration.Anchors[c.ID] {
				if p.ID == a && p.OrganizationRef == o.ID {
					anchors = append(anchors, a)
				}
			}
		}
		input["organizations"] = append(input["organizations"].([]any), Object{"MSP": o.MSPID, "Root": object(sign["status"])["ca_cert"], "TLSRoot": object(tlsCA["status"])["tlsca_cert"], "Anchors": anchors})
		for _, id := range c.OrganizationRefs {
			if id == o.ID {
				input["applicationMSPs"] = append(input["applicationMSPs"].([]string), o.MSPID)
			}
		}
	}
	for _, n := range w.channelNodes(c, "orderer") {
		v, e := w.get("fabricorderernodes", n.ID)
		if e != nil {
			return generatedConfig{}, e
		}
		status := object(v["status"])
		input["orderers"] = append(input["orderers"].([]any), Object{"Name": n.ID, "MSP": w.peerMSP(n.OrganizationRef), "TLS": status["tlsCert"], "Sign": status["signCert"]})
	}
	raw, _ := json.Marshal(input)
	cmd := exec.CommandContext(w.ctx, "fabric-config-tool")
	cmd.Stdin = bytes.NewReader(raw)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if w.execute(cmd) != nil {
		return generatedConfig{}, fmt.Errorf("protected dynamic configuration generation failed")
	}
	var result generatedConfig
	if json.Unmarshal(out.Bytes(), &result) != nil {
		return result, secureDenied
	}
	return result, nil
}

func (w *Worker) fetchDynamicBlock(c domain.Channel, peer, ordering domain.Node, which string) ([]byte, error) {
	path := "/workspace/fetched-" + c.ID + ".block"
	if _, e := w.directPeerAt(peer.OrganizationRef, "channel-admin", peer.ID, "channel", "fetch", which, path, "-c", c.ID, "-o", ordering.ID+"."+w.Namespace+":7050", "--tls", "--cafile", "/workspace/"+ordering.OrganizationRef+"-root-tls.pem"); e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, secureDenied
	}
	if _, _, e = decodeChannelConfig(raw, c.ID); e != nil {
		return nil, e
	}
	return raw, nil
}

func (w *Worker) reconcileDynamicChannel(before, desired domain.Channel) error {
	peers, orders := w.channelNodes(before, "peer"), w.channelNodes(before, "orderer")
	if len(peers) == 0 || len(orders) == 0 {
		return secureDenied
	}
	peer, ordering := peers[0], orders[0]
	for {
		current, e := w.fetchDynamicBlock(before, peer, ordering, "config")
		if e != nil {
			return e
		}
		config, _, e := decodeChannelConfig(current, desired.ID)
		if e != nil {
			return e
		}
		actual, e := dynamicConsenters(config, w.Namespace)
		if e != nil {
			return e
		}
		_, activeBlock, e := decodeChannelConfig(current, desired.ID)
		if e != nil {
			return e
		}
		active := desired
		active.OrdererRefs = append([]string(nil), actual...)
		// A retry may observe a committed admission whose join acknowledgement
		// was interrupted. Activate that consenter before proposing another one.
		for _, n := range w.channelNodes(active, "orderer") {
			if e = w.joinDynamicOrderer(n, desired.ID, current, activeBlock.Header.Number+1); e != nil {
				return e
			}
		}
		wanted := map[string]bool{}
		for _, id := range desired.OrdererRefs {
			wanted[id] = true
		}
		for _, id := range actual {
			if !wanted[id] {
				return secureDenied
			}
		}
		for _, id := range before.OrdererRefs {
			found := false
			for _, a := range actual {
				found = found || id == a
			}
			if !found {
				return secureDenied
			}
		}
		step := desired
		step.OrdererRefs = append([]string(nil), actual...)
		for _, id := range desired.OrdererRefs {
			found := false
			for _, a := range actual {
				found = found || id == a
			}
			if !found {
				step.OrdererRefs = append(step.OrdererRefs, id)
				break
			}
		}
		result, e := w.dynamicConfig(step, current)
		if e != nil {
			return e
		}
		if result.Changed {
			if len(result.Update) == 0 {
				return secureDenied
			}
			sum := sha256.Sum256(result.Update)
			checkpoint := "channel-" + desired.ID + "-u-" + hex.EncodeToString(sum[:8])
			if e = w.materialSecret(checkpoint, map[string]string{"channel": desired.ID, "update": base64.StdEncoding.EncodeToString(result.Update)}, "issuer", true); e != nil {
				return e
			}
			path := "/workspace/" + desired.ID + "-update.pb"
			if e = privateFile(path, result.Update); e != nil {
				return e
			}
			// Sign with every currently authorized organization. New organizations
			// never authorize their own admission to a channel.
			msps := map[string]bool{}
			for _, group := range []string{"Application", "Orderer"} {
				g := config.ChannelGroup.Groups[group]
				if g == nil {
					return secureDenied
				}
				for msp := range g.Groups {
					msps[msp] = true
				}
			}
			for _, org := range w.Spec.Spec.Organizations {
				if !msps[org.MSPID] {
					continue
				}
				if _, e = w.directPeerAt(org.ID, "channel-admin", w.firstPeer(org.ID), "channel", "signconfigtx", "-f", path); e != nil {
					return e
				}
				delete(msps, org.MSPID)
			}
			if len(msps) != 0 {
				return secureDenied
			}
			if _, e = w.directPeerAt(peer.OrganizationRef, "channel-admin", peer.ID, "channel", "update", "-f", path, "-c", desired.ID, "-o", ordering.ID+"."+w.Namespace+":7050", "--tls", "--cafile", "/workspace/"+ordering.OrganizationRef+"-root-tls.pem"); e != nil {
				return e
			}
			if e = w.event("channels", "Submitted authorized additive configuration checkpoint "+hex.EncodeToString(sum[:])); e != nil {
				return e
			}
		}
		// Independently fetch the committed config. Ambiguous acknowledgement is
		// recovered by deriving the next delta from this block on retry.
		matched := false
		for attempt := 0; attempt < 45; attempt++ {
			current, e = w.fetchDynamicBlock(before, peer, ordering, "config")
			if e != nil {
				return e
			}
			proof, e := w.dynamicConfig(step, current)
			if e != nil {
				return e
			}
			if !proof.Changed {
				matched = true
				break
			}
			select {
			case <-w.ctx.Done():
				return w.ctx.Err()
			case <-time.After(time.Second):
			}
		}
		if !matched {
			return fmt.Errorf("committed dynamic channel configuration not acknowledged")
		}
		_, block, e := decodeChannelConfig(current, desired.ID)
		if e != nil {
			return e
		}
		for _, n := range w.channelNodes(step, "orderer") {
			if e = w.joinDynamicOrderer(n, desired.ID, current, block.Header.Number+1); e != nil {
				return e
			}
		}
		if len(step.OrdererRefs) == len(desired.OrdererRefs) {
			break
		}
	}
	genesis, e := w.fetchDynamicBlock(before, peer, ordering, "0")
	if e != nil || validateGenesis(genesis, desired.ID) != nil {
		return secureDenied
	}
	stored, e := w.loadMaterial("channel-" + desired.ID + "-genesis")
	if e != nil || stored == nil {
		return secureDenied
	}
	owned, e := base64.StdEncoding.DecodeString(stored["block"])
	if e != nil {
		return secureDenied
	}
	var a, b common.Block
	if proto.Unmarshal(owned, &a) != nil || proto.Unmarshal(genesis, &b) != nil || !proto.Equal(a.Header, b.Header) || !proto.Equal(a.Data, b.Data) {
		return secureDenied
	}
	path := "/workspace/" + desired.ID + "-join.block"
	if e = privateFile(path, genesis); e != nil {
		return e
	}
	for _, n := range w.channelNodes(desired, "peer") {
		raw, e := w.directPeerAt(n.OrganizationRef, "channel-admin", n.ID, "channel", "list")
		if e != nil {
			return e
		}
		found := false
		for _, line := range strings.Split(string(raw), "\n") {
			found = found || strings.TrimSpace(line) == desired.ID
		}
		if !found {
			if _, e = w.directPeerAt(n.OrganizationRef, "channel-admin", n.ID, "channel", "join", "-b", path); e != nil {
				return e
			}
		}
	}
	configBlock, e := w.fetchDynamicBlock(before, peer, ordering, "config")
	if e != nil {
		return e
	}
	_, tip, e := decodeChannelConfig(configBlock, desired.ID)
	if e != nil {
		return e
	}
	return w.awaitDynamicPeers(desired, tip.Header.Number+1)
}

func (w *Worker) awaitDynamicPeers(c domain.Channel, height uint64) error {
	for attempt := 0; attempt < 90; attempt++ {
		var target *common.BlockchainInfo
		converged := true
		for _, n := range w.channelNodes(c, "peer") {
			raw, e := w.secureQueryAt(n.OrganizationRef, n.ID, c.ID, "qscc", "GetChainInfo", c.ID)
			if e != nil {
				converged = false
				break
			}
			var info common.BlockchainInfo
			if proto.Unmarshal(raw, &info) != nil {
				return secureDenied
			}
			if info.Height < height {
				converged = false
				break
			}
			if target == nil {
				target = &info
			} else if info.Height != target.Height || !bytes.Equal(info.CurrentBlockHash, target.CurrentBlockHash) {
				converged = false
				break
			}
		}
		if converged && target != nil {
			return nil
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("configured channel peers did not synchronize to the committed configuration")
}

func dynamicConsenters(config *common.Config, namespace string) ([]string, error) {
	if config == nil || config.ChannelGroup == nil || config.ChannelGroup.Groups["Orderer"] == nil {
		return nil, secureDenied
	}
	value := config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	if value == nil {
		return nil, secureDenied
	}
	var consensus orderer.ConsensusType
	var metadata etcdraft.ConfigMetadata
	if proto.Unmarshal(value.Value, &consensus) != nil || consensus.Type != "etcdraft" || proto.Unmarshal(consensus.Metadata, &metadata) != nil {
		return nil, secureDenied
	}
	var ids []string
	seen := map[string]bool{}
	for _, c := range metadata.Consenters {
		if c == nil || !strings.HasSuffix(c.Host, "."+namespace) || c.Port != 7050 {
			return nil, secureDenied
		}
		id := strings.TrimSuffix(c.Host, "."+namespace)
		if seen[id] {
			return nil, secureDenied
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) < 3 {
		return nil, secureDenied
	}
	return ids, nil
}

func (w *Worker) joinDynamicOrderer(n domain.Node, channel string, block []byte, height uint64) error {
	client, e := w.participation(n)
	if e != nil {
		return e
	}
	endpoint := "https://" + n.ID + "." + w.Namespace + ":7053/participation/v1/channels"
	request, _ := http.NewRequestWithContext(w.ctx, "GET", endpoint+"/"+channel, nil)
	response, e := client.Do(request)
	if e != nil {
		return secureDenied
	}
	status := response.StatusCode
	response.Body.Close()
	if status == 404 {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("config-block", "channel.block")
		if e != nil {
			return secureDenied
		}
		if _, e = part.Write(block); e != nil {
			return secureDenied
		}
		if writer.Close() != nil {
			return secureDenied
		}
		request, _ = http.NewRequestWithContext(w.ctx, "POST", endpoint, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response, e = client.Do(request)
		if e != nil {
			return secureDenied
		}
		response.Body.Close()
		if response.StatusCode != 201 {
			return secureDenied
		}
	} else if status != 200 {
		return secureDenied
	}
	for attempt := 0; attempt < 90; attempt++ {
		request, _ = http.NewRequestWithContext(w.ctx, "GET", endpoint+"/"+channel, nil)
		response, e = client.Do(request)
		if e != nil {
			return secureDenied
		}
		var state struct {
			Status   string `json:"status"`
			Relation string `json:"consensusRelation"`
			Height   uint64 `json:"height"`
		}
		e = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&state)
		response.Body.Close()
		if e != nil || response.StatusCode != 200 {
			return secureDenied
		}
		if state.Status == "active" && state.Relation == "consenter" && state.Height >= height {
			return nil
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("new Raft consenter did not become active and caught up before the next membership change")
}
