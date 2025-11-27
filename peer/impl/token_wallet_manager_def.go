package impl

type ITokenWalletManager interface {
	GetBalance(from string) uint64
	ChargeAndGet(from string, amount uint64) (uint64, error)
	VerifyOwnership(from string, publicKey []byte) error
	SetBalance(from string, amount uint64) uint64
}
