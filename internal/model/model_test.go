package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRecordPreservesPayload(t *testing.T) {
	r, err := ParseRecord([]byte(` { "id": "evt-1", "count": 9007199254740993, "nested": {"ok":true} } `))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"evt-1","count":9007199254740993,"nested":{"ok":true}}`
	if r.ID != "evt-1" || string(r.JSON) != want {
		t.Fatalf("got %q, %s", r.ID, r.JSON)
	}
}

func TestRejectAmbiguousOrInvalidRecords(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{}`, `{"id":null}`, `{"id":7}`, `{"id":"  "}`, `{"id":"a","id":"b"}`, `{"id":"a","x":1,"x":2}`, `{"id":"a"} {}`, `{"id":"a"} garbage`, `{"id":"` + strings.Repeat("a", 257) + `"}`, "{\"id\":\"\xff\"}", `{"id":"a"`, `{"id":"a",}`} {
		t.Run(raw[:min(len(raw), 40)], func(t *testing.T) {
			if _, err := ParseRecord([]byte(raw)); err == nil {
				t.Fatal("accepted invalid record")
			}
		})
	}
}

func TestOptions(t *testing.T) {
	o := DefaultOptions()
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Options{{"drop", "fail", 10}, {"fail", "last", 10}, {"fail", "fail", 0}, {"fail", "fail", MaxRecordBytes + 1}} {
		if invalid.Validate() == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
}

func FuzzParseRecord(f *testing.F) {
	for _, seed := range []string{`{"id":"a"}`, `{"id":"\u0061","n":1e400}`, `{"id":"a","nested":[1,null,{}]}`, `{"id":"a","id":"b"}`, "\xff"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := ParseRecord(data)
		if err != nil {
			return
		}
		if !json.Valid(r.JSON) || r.ID == "" {
			t.Fatal("accepted record has invalid output")
		}
		again, err := ParseRecord(r.JSON)
		if err != nil || again.ID != r.ID || string(again.JSON) != string(r.JSON) {
			t.Fatal("compaction is not stable")
		}
	})
}
