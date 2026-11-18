package tikidetail

import "testing"

func TestAddIDTokensFindsBothIDForms(t *testing.T) {
	ids := map[string]struct{}{}
	addIDTokens(ids, "see MEM-DB-35, x7f4k2 and [[FEAT-16]]; not MEM-0")
	for _, want := range []string{"MEM-DB-35", "X7F4K2", "FEAT-16"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("missing %s in %v", want, ids)
		}
	}
	if _, ok := ids["MEM-0"]; ok {
		t.Errorf("MEM-0 is not a valid id but was collected: %v", ids)
	}
}
