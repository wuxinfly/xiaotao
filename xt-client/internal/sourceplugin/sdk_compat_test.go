package sourceplugin

import (
	"encoding/json"
	"reflect"
	"testing"

	"xiaotao/xt-client/sdk/plugin"
)

// The SDK is a public mirror of the wire types. A drift here would let plugin
// authors compile against fields the host never sends.
func TestSDKMatchesHostContract(t *testing.T) {
	pairs := []struct {
		name string
		host any
		sdk  any
	}{
		{"Operation", Operation{}, plugin.Operation{}},
		{"Model", Model{}, plugin.Model{}},
		{"AccountInfo", AccountInfo{}, plugin.AccountInfo{}},
		{"Result", Result{}, plugin.Result{}},
		{"Display", Display{}, plugin.Display{}},
		{"DisplayItem", DisplayItem{}, plugin.DisplayItem{}},
		{"LoginDescriptor", LoginDescriptor{}, plugin.LoginDescriptor{}},
	}
	for _, pair := range pairs {
		host := jsonTags(reflect.TypeOf(pair.host))
		external := jsonTags(reflect.TypeOf(pair.sdk))
		if !reflect.DeepEqual(host, external) {
			t.Fatalf("%s drifted:\nhost=%v\nsdk =%v", pair.name, host, external)
		}
	}
	if plugin.ContractVersion != ContractVersion || plugin.MaxMessage != MaxMessage {
		t.Fatalf("constants drifted: %d/%d", plugin.ContractVersion, plugin.MaxMessage)
	}
	if plugin.CodeAuthExpired != RPCAuthExpired || plugin.CodeUnsupported != RPCUnsupported || plugin.CodeTimeout != RPCTimeout {
		t.Fatal("error codes drifted")
	}
}

func jsonTags(value reflect.Type) map[string]string {
	out := map[string]string{}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		out[field.Name] = field.Tag.Get("json")
	}
	return out
}

// A result produced by the SDK must decode into the host type and vice versa.
func TestSDKResultRoundTripsIntoHost(t *testing.T) {
	percent := 12.5
	encoded, err := json.Marshal(plugin.Result{
		Models:  []plugin.Model{{Key: "k", UpstreamID: "u", Label: "L"}},
		Display: &plugin.Display{Summary: "s", Items: []plugin.DisplayItem{{Label: "l", Value: "v", Percent: &percent}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var host Result
	if err := json.Unmarshal(encoded, &host); err != nil {
		t.Fatal(err)
	}
	if len(host.Models) != 1 || host.Models[0].Key != "k" || host.Display == nil || host.Display.Items[0].Percent == nil || *host.Display.Items[0].Percent != 12.5 {
		t.Fatalf("round trip lost data: %+v", host)
	}
}
