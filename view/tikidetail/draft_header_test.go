package tikidetail

import (
	"regexp"
	"strings"
	"testing"

	"github.com/boolean-maybe/tiki/controller"
	"github.com/boolean-maybe/tiki/store"
	tikipkg "github.com/boolean-maybe/tiki/tiki"
	"github.com/boolean-maybe/tiki/workflow"
)

var colorTag = regexp.MustCompile(`\[[^\[\]]*\]`)

func frameTitleText(cv *ConfigurableDetailView) string {
	if cv.metaFrame == nil {
		return ""
	}
	return strings.TrimSpace(colorTag.ReplaceAllString(cv.metaFrame.GetTitle(), ""))
}

func TestDraftHeaderShowsNewAndTheLivePrefix(t *testing.T) {
	registerImmutableFields(t)
	workflow.SetProjectPrefix("TIK")
	t.Cleanup(func() { workflow.SetProjectPrefix("") })

	s := store.NewInMemoryStore()
	draft := tikipkg.New()
	draft.SetID("J5RSFP")
	draft.SetProvisionalID(true)
	draft.SetTitle("draft")
	cv := NewConfigurableDetailView(s, draft.ID(), detailPluginFromFields([]string{"status", "area"}),
		controller.DetailViewActions(), nil, nil, nil, nil)
	cv.SetFallbackTiki(draft)
	cv.refresh()
	if got := frameTitleText(cv); got != "NEW · TIK" {
		t.Errorf("header before choosing area = %q, want %q", got, "NEW · TIK")
	}

	draft.Set("area", "feature")
	cv.updateDraftHeader()
	if got := frameTitleText(cv); got != "NEW · FEAT" {
		t.Errorf("header after area=feature = %q, want %q", got, "NEW · FEAT")
	}
}

func TestSavedTikiHeaderShowsItsID(t *testing.T) {
	registerImmutableFields(t)
	workflow.SetProjectPrefix("TIK")
	t.Cleanup(func() { workflow.SetProjectPrefix("") })
	s := store.NewInMemoryStore()
	storedTiki(t, s, "FEAT-3", map[string]string{"area": "feature"})
	cv := NewConfigurableDetailView(s, "FEAT-3", detailPluginFromFields([]string{"status", "area"}),
		controller.DetailViewActions(), nil, nil, nil, nil)
	cv.refresh()
	if got := frameTitleText(cv); got != "FEAT-3" {
		t.Errorf("header = %q, want FEAT-3", got)
	}
}
