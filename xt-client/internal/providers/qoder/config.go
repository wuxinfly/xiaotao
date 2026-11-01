package qoder

import (
	"strings"
)

const (
	// COSYVersion is the CLI version expected by Qoder's COSY endpoints.
	COSYVersion = "1.1.57"

	// ClientID is used by device authorization flow across CN and Global.
	ClientID = "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb"

	// ClientIDAlt is a secondary client ID observed in Qoder clients.
	ClientIDAlt = "e93fe488-5778-4c35-a6fc-0f54ed7b3139"

	// COSYRSAPublicKeyPEM is the 1024-bit RSA public key used for Cosy-Key encryption.
	COSYRSAPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

	// CustomB64Alphabet is the custom substitution alphabet for COSY body encoding.
	CustomB64Alphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"

	// StdB64Alphabet is standard RFC 4648 Base64 alphabet.
	StdB64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

// Region defines endpoint URLs and configuration for a Qoder service region.
type Region struct {
	Key           string
	Label         string
	InferBase     string
	OpenAPIBase   string
	AuthBase      string
	ConfigDirname string
}

func (r Region) ModelListURL() string {
	return r.InferBase + "/algo/api/v2/model/list?Encode=1"
}

func (r Region) ChatURL() string {
	return r.InferBase + "/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
}

func (r Region) NativeChatURL() string {
	return r.InferBase + "/model/v1/chat/completions"
}

func (r Region) QuotaURL() string {
	return r.OpenAPIBase + "/api/v2/quota/usage"
}

func (r Region) UserInfoURL() string {
	return r.OpenAPIBase + "/api/v1/userinfo"
}

func (r Region) DevicePollURL() string {
	return r.OpenAPIBase + "/api/v1/deviceToken/poll"
}

func (r Region) DeviceRefreshURL() string {
	return r.OpenAPIBase + "/api/v1/deviceToken/refresh"
}

func (r Region) JobTokenExchangeURL() string {
	return r.OpenAPIBase + "/api/v1/jobToken/exchange"
}

func (r Region) AuthURL() string {
	return r.AuthBase + "/device/selectAccounts"
}

var Regions = map[string]Region{
	"cn": {
		Key:           "cn",
		Label:         "Qoder CN",
		InferBase:     "https://gateway.qoder.com.cn",
		OpenAPIBase:   "https://openapi.qoder.com.cn",
		AuthBase:      "https://qoder.com.cn",
		ConfigDirname: ".qoder-cn",
	},
	"global": {
		Key:           "global",
		Label:         "Qoder Global",
		InferBase:     "https://api3.qoder.sh",
		OpenAPIBase:   "https://openapi.qoder.sh",
		AuthBase:      "https://qoder.com",
		ConfigDirname: ".qoder",
	},
}

// ResolveRegion returns the Region for the given key, falling back to "cn".
func ResolveRegion(key string) Region {
	k := strings.ToLower(strings.TrimSpace(key))
	if reg, ok := Regions[k]; ok {
		return reg
	}
	return Regions["cn"]
}
