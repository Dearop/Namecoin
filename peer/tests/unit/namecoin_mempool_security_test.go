package unit

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/types"

	"github.com/stretchr/testify/require"
)

func Test_HandleNamecoinTransactionMessage_AllowsInvalidTxIntoMempool(t *testing.T) {
	attacker := "attacker"

	// Craft a signed tx that will fail semantic checks once inputs are derived:
	// name_new requires at least one input UTXO, but amount=0 leads to none.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	from := hex.EncodeToString(impl.Hash(pub))
	signed := buildSignedTransaction(
		t,
		impl.NewState(),
		from,
		priv,
		impl.NameNew{}.Name(),
		0,
		impl.NameNew{Commitment: "commitment"},
		pub,
	)

	// Show that the proper semantic validation would reject the derived tx with no inputs.
	st := impl.NewState()
	svc := impl.NewTransactionService(st)
	require.Error(t, svc.ValidateTxCommand(&types.Tx{
		From:    from,
		Type:    signed.Type,
		Amount:  signed.Amount,
		Payload: signed.Payload,
	}))

	// Spin up two nodes to mimic gossip, with trivial PoW so mining is near-instant.
	trans := channel.NewTransport()
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 256) // trivially easy PoW
	nodeA := z.NewTestNode(
		t,
		peerFac,
		trans,
		"nodeA:0",
		z.WithEnableMiner(false),
		z.WithPoWConfig(peer.PoWConfig{
			Target:   easyTarget,
			MaxNonce: 0, // unlimited, but target is trivial
			PubKey:   "minerA",
		}),
	)
	nodeB := z.NewTestNode(
		t,
		peerFac,
		trans,
		"nodeB:0",
		z.WithEnableMiner(false),
		z.WithPoWConfig(peer.PoWConfig{
			Target:   easyTarget,
			MaxNonce: 0,
			PubKey:   "minerB",
		}),
	)
	defer nodeA.StopAll()
	defer nodeB.StopAll()

	// Inject the tx into nodeA as if gossiped from nodeB (bypasses TransactionService).
	regA := nodeA.GetRegistry()
	wire, err := regA.MarshalMessage(types.NamecoinTransactionMessage{
		Type:      signed.Type,
		From:      signed.From,
		Amount:    signed.Amount,
		Payload:   signed.Payload,
		Inputs:    signed.Inputs,
		Outputs:   signed.Outputs,
		Pk:        signed.Pk,
		TxID:      signed.TxID,
		Signature: signed.Signature,
	})
	require.NoError(t, err)
	header := transport.NewHeader(nodeB.GetAddr(), nodeB.GetAddr(), nodeA.GetAddr())
	require.Error(t, regA.ProcessPacket(transport.Packet{Header: &header, Msg: &wire}))

	// Start mining on nodeA.
	if miner, ok := nodeA.Peer.(interface {
		EnableMiner()
		StartMiner()
		StopMiner()
	}); ok {
		miner.EnableMiner()
		miner.StartMiner()
		defer miner.StopMiner()
	} else {
		t.Fatalf("peer does not expose miner controls")
	}

	// If the system were secure, the attacker UTXO would never appear.
	evilTxID := signed.TxID
	require.Never(t, func() bool {
		chain := nodeA.NamecoinChainState()
		if chain == nil {
			return false
		}
		state := chain.State()
		utxos := state.UTXOMap[attacker]
		if utxos == nil {
			return false
		}
		_, ok := utxos[evilTxID]
		return ok
	}, 3*time.Second, 50*time.Millisecond, "insecure: invalid tx from gossip was mined and credited")
}

func Test_HandleNamecoinTransactionMessage_AcceptsValidTxIntoMempool(t *testing.T) {
	// Funded sender submits a valid name_new (has inputs after balance selection).
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	from := hex.EncodeToString(impl.Hash(pub))
	st := impl.NewState()
	st.UTXOMap[from] = map[string]types.UTXO{
		"funds-1": {TxID: "funds-1", To: from, Amount: 10},
	}
	signed := buildSignedTransaction(
		t,
		st,
		from,
		priv,
		impl.NameNew{}.Name(),
		1,
		impl.NameNew{Commitment: "commitment-ok"},
		pub,
	)

	trans := channel.NewTransport()
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	node := z.NewTestNode(
		t,
		peerFac,
		trans,
		"node:0",
		z.WithEnableMiner(false),
		z.WithPoWConfig(peer.PoWConfig{
			Target:   easyTarget,
			MaxNonce: 0,
			PubKey:   "miner",
		}),
	)
	defer node.StopAll()

	// Seed funds so VerifyBalance succeeds on ingress.
	chain := node.NamecoinChainState()
	require.NotNil(t, chain)
	state := chain.State()
	require.NotNil(t, state)
	state.UTXOMap[from] = map[string]types.UTXO{
		"funds-1": {TxID: "funds-1", To: from, Amount: 10},
	}

	// Inject the validated tx via gossip handler.
	reg := node.GetRegistry()
	wire, err := reg.MarshalMessage(types.NamecoinTransactionMessage{
		Type:      signed.Type,
		From:      signed.From,
		Amount:    signed.Amount,
		Payload:   signed.Payload,
		Inputs:    signed.Inputs,
		Outputs:   signed.Outputs,
		Pk:        signed.Pk,
		TxID:      signed.TxID,
		Signature: signed.Signature,
	})
	require.NoError(t, err)
	header := transport.NewHeader(node.GetAddr(), node.GetAddr(), node.GetAddr())
	require.NoError(t, reg.ProcessPacket(transport.Packet{Header: &header, Msg: &wire}))

	// Start mining to include it in a block.
	if miner, ok := node.Peer.(interface {
		EnableMiner()
		StartMiner()
		StopMiner()
	}); ok {
		miner.EnableMiner()
		miner.StartMiner()
		defer miner.StopMiner()
	} else {
		t.Fatalf("peer does not expose miner controls")
	}

	txID := signed.TxID

	// Wait until the change UTXO shows up, meaning the tx was accepted and mined.
	require.Eventually(t, func() bool {
		chain := node.NamecoinChainState()
		if chain == nil {
			return false
		}
		state := chain.State()
		utxos := state.UTXOMap[from]
		if utxos == nil {
			return false
		}
		_, ok := utxos[txID]
		return ok
	}, 3*time.Second, 50*time.Millisecond, "expected mined block with validated tx")

	chain = node.NamecoinChainState()
	state = chain.State()
	utxo := state.UTXOMap[from][txID]
	require.Equal(t, uint64(9), utxo.Amount)
}
