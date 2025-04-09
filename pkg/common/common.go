package common

// SignalType
/*
Offer Answer Candidate
SignalMsg{
	Type,
	string(marshal(TargetedMsg{
		Src, Dst, string(marshal(webrtc.xx))
	}))
}

Register
SignalMsg{
	Type,
	string(marshal(RegisterMsg{
		Id
	}))
}
*/
type SignalType int

const (
	Offer SignalType = iota
	Answer
	Candidate
	Register
)

type SignalMsg struct {
	Type SignalType `json:"type"`
	Data string     `json:"data"`
}

// todo: 把targeted和offer这些都改成继承会不会好一点

type TargetedMsg struct {
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	Data string `json:"data"`
}

//type OfferMsg webrtc.SessionDescription
//
//type AnswerMsg webrtc.SessionDescription
//
//type CandidateMsg webrtc.ICECandidateInit

type RegisterMsg struct {
	Id string `json:"id"`
}
