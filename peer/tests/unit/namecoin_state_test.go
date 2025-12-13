package unit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func mustPayload(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return json.RawMessage(b)
}

func mustTxID(t *testing.T, tx *types.Tx) string {
	t.Helper()
	id, err := impl.BuildTransactionID(tx)
	require.NoError(t, err)
	return id
}

func outpointKeyForTx(t *testing.T, txID string, index uint32) string {
	t.Helper()
	return impl.OutpointKey(txID, index)
}

func Test_Namecoin_State_ApplyTx_NameNewSetsCommitmentOnly(t *testing.T) {
	st := impl.NewState()
	from := "addr1"
	payload := impl.NameNew{Commitment: "commit-123"}

	tx := types.Tx{
		From:    from,
		Type:    impl.NameNew{}.Name(),
		Payload: mustPayload(t, payload),
	}
	txID := mustTxID(t, &tx)

	require.Empty(t, st.Commitments)
	require.False(t, st.IsDomainExists("example.bit"))

	err := st.ApplyTx(txID, &tx)
	require.NoError(t, err)

	key := outpointKeyForTx(t, txID, 0)
	commit, ok := st.GetCommitment(key)
	require.True(t, ok)
	require.Equal(t, "commit-123", commit)
	require.False(t, st.IsDomainExists("example.bit"))
	require.True(t, st.IsTxApplied(txID))
}

func Test_Namecoin_State_ApplyTx_FirstUpdateCreatesDomainAndUpdatesUTXO(t *testing.T) {
	st := impl.NewState()
	from := "addr1"
	domain := "example.bit"
	salt := "secret"
	ip := "10.0.0.1"

	// Commitment format used by NameFirstUpdate and the frontend.
	commitment := impl.HashString("DOMAIN_HASH_v1:" + domain + ":" + salt)

	// Simulate the NameNew transaction that would have created the commitment
	// and its corresponding UTXO. We do NOT change core code here; we only
	// let the implementation compute and store the same txID and commitment.
	nameNewTx := types.Tx{
		From:   from,
		Type:   impl.NameNew{}.Name(),
		Amount: 1,
		Payload: mustPayload(t, impl.NameNew{
			Commitment: commitment,
			TTL:        impl.DefaultDomainTTLBlocks,
		}),
	}
	commitTxID := mustTxID(t, &nameNewTx)
	require.NoError(t, st.ApplyTx(commitTxID, &nameNewTx))
	// name_new appends an output to owner; ensure it exists for firstupdate burn.
	nameNewTx.Outputs = []types.TxOutput{{To: from, Amount: 10}}
	require.NoError(t, st.ApplyTx(commitTxID, &nameNewTx))
	commitKey := outpointKeyForTx(t, commitTxID, 0)

	// Seed one UTXO that will be spent by the firstupdate. In the real
	// system this comes from prior funding; here we just ensure the
	// UTXO map contains an entry keyed by the commit txID.
	st.UTXOMap[from] = map[string]types.UTXO{
		commitTxID: {TxID: commitTxID, To: from, Amount: 10},
	}

	payload := impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt,
		IP:     ip,
		TxID:   commitTxID,
	}

	tx := types.Tx{
		From:    from,
		Type:    impl.NameFirstUpdate{}.Name(),
		Inputs:  []types.TxInput{{TxID: commitTxID, Index: 0}},
		Outputs: []types.TxOutput{{To: from, Amount: 10}},
		Payload: mustPayload(t, payload),
	}
	txID := mustTxID(t, &tx)

	err := st.ApplyTx(txID, &tx)
	require.NoError(t, err)

	// Domain state
	rec, ok := st.Domains[domain]
	require.True(t, ok)
	require.Equal(t, from, rec.Owner)
	require.Equal(t, ip, rec.IP)
	require.Equal(t, salt, rec.Salt)

	// UTXO state: utxo-1 burned, new utxo with txID added
	userUTXOs := st.UTXOMap[from]
	require.NotNil(t, userUTXOs)
	_, ok = userUTXOs[commitTxID]
	require.False(t, ok)
	out, ok := userUTXOs[txID]
	require.True(t, ok)
	require.Equal(t, uint64(10), out.Amount)

	_, ok = st.GetCommitment(commitKey)
	require.False(t, ok, "commitment should be consumed after first update")
}

func Test_Namecoin_State_ApplyTx_NameUpdateChangesIP(t *testing.T) {
	st := impl.NewState()
	from := "owner"
	domain := "foo.bit"

	st.Domains[domain] = types.NameRecord{
		Owner:  from,
		Domain: domain,
		IP:     "1.1.1.1",
	}

	payload := impl.NameUpdate{
		Domain: domain,
		IP:     "2.2.2.2",
	}

	tx := types.Tx{
		From:    from,
		Type:    impl.NameUpdate{}.Name(),
		Payload: mustPayload(t, payload),
	}
	txID := mustTxID(t, &tx)

	err := st.ApplyTx(txID, &tx)
	require.NoError(t, err)

	rec, ok := st.Domains[domain]
	require.True(t, ok)
	require.Equal(t, "2.2.2.2", rec.IP)
	require.Equal(t, domain, rec.Domain) // unchanged
}

func Test_Namecoin_State_ApplyTx_RewardCreatesMinerUTXO(t *testing.T) {
	st := impl.NewState()
	miner := "miner-addr"

	tx := types.Tx{
		Type: impl.Reward{}.Name(),
		Outputs: []types.TxOutput{{
			To:     miner,
			Amount: 50,
		}},
	}
	txID := mustTxID(t, &tx)

	err := st.ApplyTx(txID, &tx)
	require.NoError(t, err)

	utxos := st.UTXOMap[miner]
	require.NotNil(t, utxos)
	u, ok := utxos[txID]
	require.True(t, ok)
	require.Equal(t, uint64(50), u.Amount)
}

func Test_Namecoin_State_ApplyTx_IsIdempotentOnSameTxID(t *testing.T) {
	st := impl.NewState()
	from := "addr1"
	payload := impl.NameNew{Commitment: "commit-idem"}

	tx := types.Tx{
		From:    from,
		Type:    impl.NameNew{}.Name(),
		Payload: mustPayload(t, payload),
	}
	txID := mustTxID(t, &tx)

	err := st.ApplyTx(txID, &tx)
	require.NoError(t, err)
	require.True(t, st.IsTxApplied(txID))

	// Capture snapshot
	key := outpointKeyForTx(t, txID, 0)
	beforeCommit, ok := st.GetCommitment(key)
	require.True(t, ok)

	// Second apply should be a no-op
	err = st.ApplyTx(txID, &tx)
	require.NoError(t, err)
	commitAfter, ok := st.GetCommitment(key)
	require.True(t, ok)
	require.Equal(t, beforeCommit, commitAfter)
}
