// Command conformance checks a built plugin executable against the host protocol.
//
//	go run ./sdk/conformance/cmd/conformance dist/plugins/example/provider.exe example.echo
package main

import (
	"context"
	"fmt"
	"os"

	"xiaotao/xt-client/sdk/conformance"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: conformance <plugin-exe> <plugin-id>")
		os.Exit(2)
	}
	result := conformance.Run(context.Background(), os.Args[1], os.Args[2])
	for _, check := range result.Checks {
		status := "ok  "
		if check.Err != nil {
			status = "FAIL"
		}
		fmt.Printf("%s  %s", status, check.Name)
		if check.Err != nil {
			fmt.Printf(": %v", check.Err)
		}
		fmt.Println()
	}
	if !result.OK() {
		os.Exit(1)
	}
}
