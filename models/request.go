package models

type SendMessageRequest struct {
	Token   string `json:"token" form:"token"`
	Number  string `json:"number" form:"number"`
	Text    string `json:"text" form:"text"`
}

type SendMediaRequest struct {
	Token    string `form:"token"`
	Number   string `form:"number"`
	Url      string `form:"url"`
	Caption  string `form:"caption"`
	Filename string `form:"filename"`
	Type     string `form:"type"`
}

type GetGroupsRequest struct {
	Token string `form:"token"`
}

type SendPollRequest struct {
	Token     string `form:"token"`
	Number    string `form:"number"`
	Name      string `form:"name"`
	Options   string `form:"options"`
	Countable bool   `form:"countable"`
}
