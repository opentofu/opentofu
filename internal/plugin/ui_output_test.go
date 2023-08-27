// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/placeholderplaceholderplaceholder/opentf/internal/opentf"
)

func TestUIOutput_impl(t *testing.T) {
	var _ opentf.UIOutput = new(UIOutput)
}

func TestUIOutput_input(t *testing.T) {
	client, server := plugin.TestRPCConn(t)
	defer client.Close()

	o := new(opentf.MockUIOutput)

	err := server.RegisterName("Plugin", &UIOutputServer{
		UIOutput: o,
	})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	output := &UIOutput{Client: client}
	output.Output("foo")
	if !o.OutputCalled {
		t.Fatal("output should be called")
	}
	if o.OutputMessage != "foo" {
		t.Fatalf("bad: %#v", o.OutputMessage)
	}
}
