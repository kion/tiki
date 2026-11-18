package controller

import (
	"testing"

	tikipkg "github.com/boolean-maybe/tiki/tiki"
)

func TestSortTikisByTitle_TiesBreakInNaturalIDOrder(t *testing.T) {
	mk := func(id string) *tikipkg.Tiki {
		tk := tikipkg.New()
		tk.SetID(id)
		tk.SetTitle("same")
		return tk
	}
	tikis := []*tikipkg.Tiki{mk("MEM-10"), mk("MEM-9")}
	sortTikisByTitle(tikis)
	if tikis[0].ID() != "MEM-9" {
		t.Fatalf("order = %s, %s; want MEM-9 first", tikis[0].ID(), tikis[1].ID())
	}
}
