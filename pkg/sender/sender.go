package sender

// Sender interface
type Sender interface {
	Send(dst string, data []byte)
	Listen()
	Accept()
	GetConnectionType(dst string) (typeString string, ok bool)
}
