package sender

// Sender interface
type Sender interface {
	Send(dst string, data string)
	Listen()
	Accept()
}
