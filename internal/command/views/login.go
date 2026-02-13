// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Login interface {
	Diagnostics(diags tfdiags.Diagnostics)
	HelpPrompt(credentialsFile string)

	UiSeparator()
	MOTDMessage(msg string)
	DefaultTFCLoginSuccess()
	DefaultTFELoginSuccess(dispHostname string)
	TokenObtainedConfirmation(dispHostname string)
	OpeningBrowserForOAuth(dispHostname, authCodeURL string)
	ManualBrowserLaunch(dispHostname, authCodeURL string)
	WaitingForHostSignal()
	PasswordRequestHeader()
	OpeningBrowserForTokens(dispHostname, tokensURL string)
	ManualBrowserLaunchForTokens(dispHostname, tokensURL string)
	GenerateTokenInstruction()
	TokenStorageInFile(localFilename string)
	TokenStorageInHelper(helperType string)
	RetrievedTokenForUser(username string)
	RequestAPITokenMessage(dispHostname, mechanism string)
	BrowserBasedLoginInstruction()
	StorageLocationConsentInHelper(helperType string)
	StorageLocationConsentInFile(localFilename string)
}

// NewLogin returns an initialized Login implementation for the given ViewType.
func NewLogin(args arguments.ViewOptions, view *View) Login {
	var ret Login
	switch args.ViewType {
	case arguments.ViewJSON:
		ret = &LoginJSON{view: NewJSONView(view, nil)}
	case arguments.ViewHuman:
		ret = &LoginHuman{view: view}
	default:
		panic(fmt.Sprintf("unknown view type %v", args.ViewType))
	}

	if args.JSONInto != nil {
		ret = &LoginMulti{ret, &LoginJSON{view: NewJSONView(view, args.JSONInto)}}
	}
	return ret
}

type LoginMulti []Login

var _ Login = (LoginMulti)(nil)

func (m LoginMulti) Diagnostics(diags tfdiags.Diagnostics) {
	for _, o := range m {
		o.Diagnostics(diags)
	}
}

func (m LoginMulti) HelpPrompt(credentialsFile string) {
	for _, o := range m {
		o.HelpPrompt(credentialsFile)
	}
}

func (m LoginMulti) UiSeparator() {
	for _, o := range m {
		o.UiSeparator()
	}
}

func (m LoginMulti) MOTDMessage(msg string) {
	for _, o := range m {
		o.MOTDMessage(msg)
	}
}

func (m LoginMulti) DefaultTFCLoginSuccess() {
	for _, o := range m {
		o.DefaultTFCLoginSuccess()
	}
}

func (m LoginMulti) DefaultTFELoginSuccess(dispHostname string) {
	for _, o := range m {
		o.DefaultTFELoginSuccess(dispHostname)
	}
}

func (m LoginMulti) TokenObtainedConfirmation(dispHostname string) {
	for _, o := range m {
		o.TokenObtainedConfirmation(dispHostname)
	}
}

func (m LoginMulti) OpeningBrowserForOAuth(dispHostname, authCodeURL string) {
	for _, o := range m {
		o.OpeningBrowserForOAuth(dispHostname, authCodeURL)
	}
}

func (m LoginMulti) ManualBrowserLaunch(dispHostname, authCodeURL string) {
	for _, o := range m {
		o.ManualBrowserLaunch(dispHostname, authCodeURL)
	}
}

func (m LoginMulti) WaitingForHostSignal() {
	for _, o := range m {
		o.WaitingForHostSignal()
	}
}

func (m LoginMulti) PasswordRequestHeader() {
	for _, o := range m {
		o.PasswordRequestHeader()
	}
}

func (m LoginMulti) OpeningBrowserForTokens(dispHostname, tokensURL string) {
	for _, o := range m {
		o.OpeningBrowserForTokens(dispHostname, tokensURL)
	}
}

func (m LoginMulti) ManualBrowserLaunchForTokens(dispHostname, tokensURL string) {
	for _, o := range m {
		o.ManualBrowserLaunchForTokens(dispHostname, tokensURL)
	}
}

func (m LoginMulti) GenerateTokenInstruction() {
	for _, o := range m {
		o.GenerateTokenInstruction()
	}
}

func (m LoginMulti) TokenStorageInFile(localFilename string) {
	for _, o := range m {
		o.TokenStorageInFile(localFilename)
	}
}

func (m LoginMulti) TokenStorageInHelper(helperType string) {
	for _, o := range m {
		o.TokenStorageInHelper(helperType)
	}
}

func (m LoginMulti) RetrievedTokenForUser(username string) {
	for _, o := range m {
		o.RetrievedTokenForUser(username)
	}
}

func (m LoginMulti) RequestAPITokenMessage(dispHostname, mechanism string) {
	for _, o := range m {
		o.RequestAPITokenMessage(dispHostname, mechanism)
	}
}

func (m LoginMulti) BrowserBasedLoginInstruction() {
	for _, o := range m {
		o.BrowserBasedLoginInstruction()
	}
}

func (m LoginMulti) StorageLocationConsentInHelper(helperType string) {
	for _, o := range m {
		o.StorageLocationConsentInHelper(helperType)
	}
}

func (m LoginMulti) StorageLocationConsentInFile(localFilename string) {
	for _, o := range m {
		o.StorageLocationConsentInFile(localFilename)
	}
}

type LoginHuman struct {
	view *View
}

var _ Login = (*LoginHuman)(nil)

func (v *LoginHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *LoginHuman) HelpPrompt(credentialsFile string) {
	helpText := fmt.Sprintf(`
Usage: tofu [global options] login [hostname]

  Retrieves an authentication token for the given hostname, if it supports
  automatic login, and saves it in a credentials file in your home directory.

  If not overridden by credentials helper settings in the CLI configuration,
  the credentials will be written to the following local file:
      %s
`, credentialsFile)
	_, _ = v.view.streams.Eprintln(helpText)
}

func (v *LoginHuman) UiSeparator() {
	msg := "\n---------------------------------------------------------------------------------\n"
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) MOTDMessage(msg string) {
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *LoginHuman) DefaultTFCLoginSuccess() {
	const msg = "\n[green][bold]Success![reset] [bold]Logged in to cloud backend[reset]\n"
	_, _ = v.view.streams.Println(v.view.colorize.Color(msg))
}

func (v *LoginHuman) DefaultTFELoginSuccess(dispHostname string) {
	const msg = "[green][bold]Success![reset] [bold]Logged in to the cloud backend (%s)[reset]\n"
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), dispHostname)
	_, _ = v.view.streams.Println(colorisedMsg)
}

func (v *LoginHuman) TokenObtainedConfirmation(dispHostname string) {
	const msg = `[green][bold]Success![reset] [bold]OpenTofu has obtained and saved an API token.[reset]

The new API token will be used for any future OpenTofu command that must make
authenticated requests to %s.%s`
	colorisedMsg := fmt.Sprintf(v.view.colorize.Color(msg), dispHostname, "\n")
	_, _ = v.view.streams.Println(colorisedMsg)
}

func (v *LoginHuman) OpeningBrowserForOAuth(dispHostname, authCodeURL string) {
	msg := fmt.Sprintf("OpenTofu must now open a web browser to the login page for %s.\n", dispHostname)
	_, _ = v.view.streams.Println(msg)
	msg2 := fmt.Sprintf("If a browser does not open this automatically, open the following URL to proceed:\n    %s\n", authCodeURL)
	_, _ = v.view.streams.Println(msg2)
}

func (v *LoginHuman) ManualBrowserLaunch(dispHostname, authCodeURL string) {
	msg := fmt.Sprintf("Open the following URL to access the login page for %s:\n    %s\n", dispHostname, authCodeURL)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) WaitingForHostSignal() {
	msg := "OpenTofu will now wait for the host to signal that login was successful.\n"
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) PasswordRequestHeader() {
	msg := "OpenTofu must temporarily use your password to request an API token.\nThis password will NOT be saved locally.\n"
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) OpeningBrowserForTokens(dispHostname, tokensURL string) {
	msg := fmt.Sprintf("OpenTofu must now open a web browser to the tokens page for %s.\n", dispHostname)
	_, _ = v.view.streams.Println(msg)
	msg2 := fmt.Sprintf("If a browser does not open this automatically, open the following URL to proceed:\n    %s\n", tokensURL)
	_, _ = v.view.streams.Println(msg2)
}

func (v *LoginHuman) ManualBrowserLaunchForTokens(dispHostname, tokensURL string) {
	msg := fmt.Sprintf("Open the following URL to access the tokens page for %s:\n    %s\n", dispHostname, tokensURL)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) GenerateTokenInstruction() {
	msg := "Generate a token using your browser, and copy-paste it into this prompt.\n"
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) TokenStorageInHelper(helperType string) {
	msg := fmt.Sprintf("OpenTofu will store the token in the configured %q credentials helper\nfor use by subsequent commands.\n", helperType)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) TokenStorageInFile(localFilename string) {
	msg := fmt.Sprintf("OpenTofu will store the token in plain text in the following file\nfor use by subsequent commands:\n    %s\n", localFilename)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) RetrievedTokenForUser(username string) {
	msg := fmt.Sprintf(v.view.colorize.Color("\nRetrieved token for user [bold]%s[reset]\n"), username)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) RequestAPITokenMessage(dispHostname, mechanism string) {
	msg := fmt.Sprintf("OpenTofu will request an API token for %s using %s.\n", dispHostname, mechanism)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) BrowserBasedLoginInstruction() {
	msg := "This will work only if you are able to use a web browser on this computer to\ncomplete a login process. If not, you must obtain an API token by another\nmeans and configure it in the CLI configuration manually.\n"
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) StorageLocationConsentInHelper(helperType string) {
	msg := fmt.Sprintf("If login is successful, OpenTofu will store the token in the configured\n%q credentials helper for use by subsequent commands.\n", helperType)
	_, _ = v.view.streams.Println(msg)
}

func (v *LoginHuman) StorageLocationConsentInFile(localFilename string) {
	msg := fmt.Sprintf("If login is successful, OpenTofu will store the token in plain text in\nthe following file for use by subsequent commands:\n    %s\n", localFilename)
	_, _ = v.view.streams.Println(msg)
}

type LoginJSON struct {
	view *JSONView
}

var _ Login = (*LoginJSON)(nil)

func (v *LoginJSON) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *LoginJSON) HelpPrompt(credentialsFile string) {}

func (v *LoginJSON) UiSeparator() {
}

func (v *LoginJSON) MOTDMessage(msg string) {
	v.view.Info(msg)
}

func (v *LoginJSON) DefaultTFCLoginSuccess() {
	v.view.Info("Success! Logged in to cloud backend")
}

func (v *LoginJSON) DefaultTFELoginSuccess(dispHostname string) {
	const msg = "Success! Logged in to the cloud backend (%s)"
	v.view.Info(fmt.Sprintf(msg, dispHostname))
}

func (v *LoginJSON) TokenObtainedConfirmation(dispHostname string) {
	const msg = `Success! OpenTofu has obtained and saved an API token. The new API token will be used for any future OpenTofu command that must make authenticated requests to %s`
	v.view.Info(fmt.Sprintf(msg, dispHostname))
}

func (v *LoginJSON) OpeningBrowserForOAuth(dispHostname, authCodeURL string) {
	msg := fmt.Sprintf("Opening web browser to the login page for %[1]s: %[2]s. If a browser does not open this automatically, open the following URL to proceed: %[2]s", dispHostname, authCodeURL)
	v.view.Info(msg)
}

func (v *LoginJSON) ManualBrowserLaunch(dispHostname, authCodeURL string) {
	v.view.Info(fmt.Sprintf("Please open the following URL to access the login page for %s: %s", dispHostname, authCodeURL))
}

func (v *LoginJSON) WaitingForHostSignal() {
	v.view.Info("Waiting for the host to signal that login was successful")
}

func (v *LoginJSON) PasswordRequestHeader() {
	v.view.Info("OpenTofu must temporarily use your password to request an API token. This password will NOT be saved locally.")
}

func (v *LoginJSON) OpeningBrowserForTokens(dispHostname, tokensURL string) {
	msg := fmt.Sprintf("OpenTofu must now open a web browser to the login page for %[1]s: %[2]s. If a browser does not open this automatically, open the following URL to proceed: %[2]s", dispHostname, tokensURL)
	v.view.Info(msg)
}

func (v *LoginJSON) ManualBrowserLaunchForTokens(dispHostname, tokensURL string) {
	v.view.Info(fmt.Sprintf("Open the following URL to access the tokens page for %s: %s", dispHostname, tokensURL))
}

func (v *LoginJSON) GenerateTokenInstruction() {
	v.view.Info("Generate a token using your browser, and copy-paste it into the prompt")
}

func (v *LoginJSON) TokenStorageInHelper(helperType string) {
	v.view.Info(fmt.Sprintf("OpenTofu will store the token in the configured %q credentials helper for use by subsequent commands", helperType))
}

func (v *LoginJSON) TokenStorageInFile(localFilename string) {
	v.view.Info(fmt.Sprintf("OpenTofu will store the token in plain text in the following file for use by subsequent commands: %s", localFilename))
}

func (v *LoginJSON) RetrievedTokenForUser(username string) {
	v.view.Info(fmt.Sprintf("Retrieved token for user %s", username))
}

func (v *LoginJSON) RequestAPITokenMessage(dispHostname, mechanism string) {
	v.view.Info(fmt.Sprintf("OpenTofu will request an API token for %s using %s", dispHostname, mechanism))
}

func (v *LoginJSON) BrowserBasedLoginInstruction() {
	v.view.Info("This will work only if you are able to use a web browser on this computer to complete a login process. If not, you must obtain an API token by another means and configure it in the CLI configuration manually.")
}

func (v *LoginJSON) StorageLocationConsentInHelper(helperType string) {
	v.view.Info(fmt.Sprintf("If login is successful, OpenTofu will store the token in the configured %q credentials helper for use by subsequent commands", helperType))
}

func (v *LoginJSON) StorageLocationConsentInFile(localFilename string) {
	v.view.Info(fmt.Sprintf("If login is successful, OpenTofu will store the token in plain text in the following file for use by subsequent commands: %s", localFilename))
}
