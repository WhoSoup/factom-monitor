package monitor

// MinuteResponse is a struct formed after the response from the factomd API
// See: https://github.com/FactomProject/factomd/blob/0ff77090ab055d4c069612ca1a6814bde88155ab/wsapi/wsapiStructs.go#L68-L79
type MinuteResponse struct {
	DBHeight        int64 `json:"directoryblockheight"`
	LeaderHeight    int64 `json:"leaderheight"`
	Minute          int64 `json:"minute"`
	BlockStartTime  int64 `json:"currentblockstarttime"`
	MinuteStartTime int64 `json:"currentminutestarttime"`
	Time            int64 `json:"currenttime"`
	DBlockSeconds   int64 `json:"directoryblockinseconds"`
}
