package e2e

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type Output struct {
	raw []byte
	t   *testing.T
}

func (o *Output) Contains(str string) *Output {
	o.t.Helper()
	if !strings.Contains(string(o.raw), str) {
		o.t.Errorf("Expected %q in output:\n %s", str, string(o.raw))
	}

	return o
}

func (o *Output) Missing(str string) *Output {
	o.t.Helper()
	if strings.Contains(string(o.raw), str) {
		o.t.Errorf("Did not expect %q in output", str)
	}

	return o
}

func (o *Output) After(str string) *Output {
	o.t.Helper()
	_, after, found := strings.Cut(string(o.raw), str)
	if !found {
		o.t.Errorf("Expected %q in output:\n %s", str, string(o.raw))
		return o
	}
	return &Output{t: o.t, raw: []byte(after)}
}

func (o *Output) JSON() *JSON {
	j := &JSON{o: o}
	messages := bytes.Split(o.raw, []byte{'\n'})
	for _, message := range messages {
		if len(message) == 0 {
			continue
		}
		var record JSONRecord
		err := json.Unmarshal(message, &record)
		if err != nil {
			o.t.Fatalf("%v: %q", err, string(message))
		}
		j.records = append(j.records, record)
	}

	return j
}

type JSONRecord struct {
	Type       string     `json:"type"`
	Diagnostic Diagnostic `json:"diagnostic"`
}
type JSON struct {
	o       *Output
	records []JSONRecord
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

func (j *JSON) HasDiagnostic(description string, valid func(Diagnostic) bool) *JSON {
	j.o.t.Helper()
	for _, record := range j.records {
		if record.Type == "diagnostic" {
			if valid(record.Diagnostic) {
				return j
			}
		}
	}
	j.o.t.Errorf("Diagnostic %s not found in:\n %s", description, string(j.o.raw))
	return j
}
