package conformance

import (
	"context"
	"os"
	"testing"

	"xiaotao/xt-client/sdk/plugin"
)

func TestMain(m *testing.M) {
	if os.Getenv("XT_CONFORMANCE_CHILD") == "1" {
		server := &plugin.Server{ID: "example.conformance", Handlers: map[string]plugin.Handler{
			"models/refresh": func(context.Context, plugin.Operation) (plugin.Result, error) {
				return plugin.Result{Models: []plugin.Model{{Key: "m", UpstreamID: "m", Label: "M"}}}, nil
			},
		}}
		server.Main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRunAcceptsSDKPlugin(t *testing.T) {
	t.Setenv("XT_CONFORMANCE_CHILD", "1")
	result := Run(context.Background(), os.Args[0], "example.conformance")
	for _, check := range result.Checks {
		if check.Err != nil {
			t.Fatalf("%s: %v", check.Name, check.Err)
		}
	}
	if len(result.Checks) < 4 || !result.OK() {
		t.Fatalf("checks incomplete: %+v", result)
	}
}

func TestRunRejectsWrongIdentity(t *testing.T) {
	t.Setenv("XT_CONFORMANCE_CHILD", "1")
	result := Run(context.Background(), os.Args[0], "example.other")
	if result.OK() {
		t.Fatal("mismatched identity passed")
	}
}

func TestRunReportsMissingExecutable(t *testing.T) {
	result := Run(context.Background(), "definitely-not-a-real-plugin.exe", "x.y")
	if result.OK() || len(result.Checks) == 0 {
		t.Fatalf("missing executable passed: %+v", result)
	}
}