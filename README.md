[TOC]

# api 介绍
```
type Sender interface {
    // 向dst发送数据，底层需要先与dst建立连接
    Send(dst string, data []byte)
    // 启动连接监听，若send的对端未启动监听，无法建立连接
    Listen()
    // 同步函数，接受连接发起者的连接
    Accept()
    // 获取当前连接到dst的连接类型
    GetConnectionType(dst string) (typeString string, ok bool)
}

func NewNatSender(source string, signalingServer string, onMessage func(msg []byte), logLevel logging.LogLevel) *NatSender
// 建立一个新的Sender，可以用于监听连接和发送数据，source为当前Sender连接标识 signalServer为连接的signaling服务器地址 onMessage为收到消息的回调函数 logLevel为日志级别
```


# 用法介绍
```
// 初始化sender
natSender := sender.NewNatSender(*source, *signalingServer, func(msg []byte) { log.Print(string(msg)) }, logging.LogLevelWarn)
// 监听并接受连接
natSender.Listen()
go func() {
	for {
		select {
		case <-natSender.Ctx.Done():
			return
		default:
		}
		natSender.Accept()
	}
}()=
// 向对端发送数据
go func() {
	for {
		for _, dst := range dsts {
			natSender.Send(dst, []byte("hello"))
		}
		select {
		case <-natSender.Ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}()
```
