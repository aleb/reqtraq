// @tests @llr REQ-0-DDLN-SWL-015
package main

import (
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/daedaleanai/reqtraq/config"
	"github.com/stretchr/testify/assert"
)

func TestReqGraph_AddCodeRef(t *testing.T) {
	rg := reqGraph{}
	const id = "certdocs/a.cc"
	rg.AddCodeRefs(id, "a.cc", "", []string{"REQ-0-DDLN-0-SWH-001"})
	v := rg["a.cc"]
	if v == nil {
		// fatal instead of error
		t.Fatalf("Failure adding code reference %q: %v", id, rg)
	}

	if v.Level != config.CODE {
		t.Errorf("expected level CODE, got %v", v.Level)
	}

	if v.Path != "a.cc" {
		t.Errorf("expected path /tmp/a.cc, got %q", v.Path)
	}
}

func TestReqGraph_AddReq(t *testing.T) {
	rg := reqGraph{}

	req := &Req{ID: "REQ-0-DDLN-SWH-001"}
	req2 := &Req{ID: "REQ-0-DDLN-SWL-001", ParentIds: []string{"REQ-0-DDLN-SWH-001"}}

	rg.AddReq(req, "./0-DDLN-0-SRD.md")
	rg.AddReq(req2, "./0-DDLN-1-SDD.md")

	// if this becomes more complex we can move it into a table of tescases.
	if expectedReq := (&Req{
		ID:   "REQ-0-DDLN-SWH-001",
		Path: "./0-DDLN-0-SRD.md",
	}); !reflect.DeepEqual(expectedReq, rg["REQ-0-DDLN-SWH-001"]) {
		t.Errorf("\nexpected %#v,\n     got %#v", expectedReq, rg["REQ-0-DDLN-SWH-001"])
	}

	if expectedReq := (&Req{
		ID:        "REQ-0-DDLN-SWL-001",
		Path:      "./0-DDLN-1-SDD.md",
		ParentIds: []string{"REQ-0-DDLN-SWH-001"},
	}); !reflect.DeepEqual(expectedReq, rg["REQ-0-DDLN-SWL-001"]) {
		t.Errorf("\nexpected %#v,\n     got %#v", expectedReq, rg["REQ-0-DDLN-SWL-001"])
	}
}

func TestReqGraph_AddReqSomeMore(t *testing.T) {
	rg := reqGraph{}

	for _, v := range []*Req{
		{ID: "REQ-0-DDLN-SWH-001", Position: 1},
		{ID: "REQ-0-DDLN-SWH-002", Position: 2},
		{ID: "REQ-0-DDLN-SWH-003", Position: 3},
	} {
		if err := rg.AddReq(v, "./0-DDLN-0-SRD.md"); err != nil {
			t.Errorf("addreq: %v", err)
		}
	}

	for _, v := range []*Req{
		{ID: "REQ-0-DDLN-SWL-001", ParentIds: []string{"REQ-0-DDLN-SWH-001"}, Position: 1},
		{ID: "REQ-0-DDLN-SWL-002", ParentIds: []string{"REQ-0-DDLN-SWH-001"}, Position: 2},
		{ID: "REQ-0-DDLN-SWL-003", ParentIds: []string{"REQ-0-DDLN-SWH-003"}, Position: 3},
	} {
		if err := rg.AddReq(v, "./0-DDLN-1-SDD.md"); err != nil {
			t.Errorf("addreq: %v", err)
		}
	}

	for i, v := range []struct {
		id     string
		expect Req
	}{
		{"REQ-0-DDLN-SWH-001", Req{ID: "REQ-0-DDLN-SWH-001", Path: "./0-DDLN-0-SRD.md", Position: 1}},
		{"REQ-0-DDLN-SWL-001", Req{
			ID:        "REQ-0-DDLN-SWL-001",
			Path:      "./0-DDLN-1-SDD.md",
			ParentIds: []string{"REQ-0-DDLN-SWH-001"},
			Position:  1,
		}},
	} {
		if !reflect.DeepEqual(v.expect, *rg[v.id]) {
			t.Errorf("case %d:\nexpected %#v,\n     got %#v", i, v.expect, *rg[v.id])
		}
	}
}

func TestReq_ReqType(t *testing.T) {
	req := Req{ID: "REQ-0-DDLN-SWL-001"}

	if v := req.ReqType(); v != "SWL" {
		t.Error("Expected SWL got", v)
	}
}

func TestReq_ReqTypeNoMatch(t *testing.T) {
	req := Req{ID: "Garbage"}

	if v := req.ReqType(); v != "" {
		t.Error("Expected nothing got", v)
	}
}

func TestReq_IdFilter(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWH-001", Body: "thrust control"}
	filter := ReqFilter{IdFilter: regexp.MustCompile("REQ-0-DDLN-SWH-*")}
	if !r.Matches(filter, nil) {
		t.Errorf("expected matching requirement but did not match")
	}
}

func TestReq_TitleFilter(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWH-001", Title: "The control unit will calculate thrust.", Body: "It will also do much more."}
	filter := ReqFilter{TitleFilter: regexp.MustCompile("thrust")}
	if !r.Matches(filter, nil) {
		t.Errorf("expected matching requirement but did not match")
	}
}

func TestReq_TitleFilterNegative(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWH-001", Title: "The control unit will calculate vertical take off speed.", Body: "It will also output thrust."}
	filter := ReqFilter{TitleFilter: regexp.MustCompile("thrust")}
	if r.Matches(filter, nil) {
		t.Errorf("expected mismatching requirement but found match")
	}
}

func TestReq_BodyFilter(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWH-001", Body: "thrust control"}
	filter := ReqFilter{BodyFilter: regexp.MustCompile("thrust")}
	if !r.Matches(filter, nil) {
		t.Errorf("expected matching requirement but did not match")
	}
}

func TestReq_IdAndBodyFilter(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWL-014", Body: "thrust control"}
	filter := ReqFilter{IdFilter: regexp.MustCompile("REQ-0-*"), BodyFilter: regexp.MustCompile("thrust")}
	if !r.Matches(filter, nil) {
		t.Errorf("expected matching requirement but did not match")
	}
}

func TestReq_IdAndBodyFilterNegative(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWL-014", Body: "thrust control"}
	filter := ReqFilter{IdFilter: regexp.MustCompile("REQ-1-*"), BodyFilter: regexp.MustCompile("thrust")}
	if r.Matches(filter, nil) {
		t.Errorf("expected mismatching requirement but found match")
	}
}

func TestReq_MatchesDiffs(t *testing.T) {
	r := Req{ID: "REQ-0-DDLN-SWL-014", Body: "thrust control"}
	// Matching filter.
	filter := ReqFilter{}
	diffs := make(map[string][]string)
	if r.Matches(filter, diffs) {
		t.Errorf("expected mismatching requirement but found match")
	}
	diffs[r.ID] = make([]string, 0)
	if !r.Matches(filter, diffs) {
		t.Errorf("expected matching requirement but found mismatch")
	}

	// Mismatching filter.
	filter[IdFilter] = regexp.MustCompile("X")
	if r.Matches(filter, diffs) {
		t.Errorf("expected mismatching requirement but found match (mismatching filter)")
	}
}

// @tests @llr REQ-0-DDLN-SWL-015
func TestParsing(t *testing.T) {
	CheckParsing(t, "testdata/valid_system_requirement/123-TEST-100-ORD.lyx")
	CheckParsing(t, "testdata/valid_system_requirement/123-TEST-100-ORD.md")
}

func CheckParsing(t *testing.T, f string) {
	rg := reqGraph{}
	errors := parseCertdocToGraph(f, rg)
	assert.Empty(t, errors, "Unexpected errors while parsing "+f)
	var systemReqs [5]Req
	for i := 0; i < 5; i++ {
		reqNo := strconv.Itoa(i + 1)
		systemReqs[i] = Req{ID: "REQ-123-TEST-SYS-00" + reqNo,
			Level:    config.SYSTEM,
			Path:     f,
			Position: i,
			Attributes: map[string]string{
				"SAFETY IMPACT": "Impact " + reqNo,
				"RATIONALE":     "Rationale " + reqNo,
				"VERIFICATION":  "Test " + reqNo},
		}
	}

	for i, systemReq := range rg.OrdsByPosition() {
		if systemReqs[i].ID != systemReq.ID || systemReqs[i].Level != systemReq.Level || systemReqs[i].Path != systemReq.Path || systemReqs[i].Position != systemReq.Position {
			t.Errorf("Invalid system requirement\nExpected %#v,\n   got %#v", systemReqs[i], systemReq)
		}
	}
}

func TestReq_IsDeleted(t *testing.T) {
	req := Req{ID: "REQ-123-TEST-SYS-002", Title: "DELETED Requirement", Body: "This is the body"}
	assert.True(t, req.IsDeleted(), "Requirement with title %s should have status DELETED", req.Body)
}
