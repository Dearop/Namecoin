package peer

type Namecoin interface {
	HandleNamecoinCommand(buf []byte) error
}
