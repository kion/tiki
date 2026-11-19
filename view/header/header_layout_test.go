package header

import (
	"testing"
)

const defaultLogoWidth = 25

func TestCalculateHeaderLayout_contextWidthPreserved(t *testing.T) {
	layout := CalculateHeaderLayout(200, 50, defaultLogoWidth)
	if layout.ContextWidth != 50 || !layout.ShowLogo {
		t.Errorf("layout = %+v, want ContextWidth 50 with logo", layout)
	}
}

func TestCalculateHeaderLayout_logoHiddenWhenHintsWouldBeSqueezed(t *testing.T) {
	layout := CalculateHeaderLayout(InfoWidth+defaultLogoWidth+MinContextWidth-1, 200, defaultLogoWidth)
	if layout.ShowLogo {
		t.Error("logo should be hidden below InfoWidth + logo + MinContextWidth")
	}
	if want := defaultLogoWidth + MinContextWidth - 1; layout.ContextWidth != want {
		t.Errorf("ContextWidth = %d, want %d (the logo's space goes to the hints)", layout.ContextWidth, want)
	}
}

func TestCalculateHeaderLayout_logoShownAtThreshold(t *testing.T) {
	layout := CalculateHeaderLayout(InfoWidth+defaultLogoWidth+MinContextWidth, 200, defaultLogoWidth)
	if !layout.ShowLogo || layout.ContextWidth != MinContextWidth {
		t.Errorf("layout = %+v, want logo shown and %d columns of hints", layout, MinContextWidth)
	}
}

func TestCalculateHeaderLayout_eightyColumns(t *testing.T) {
	layout := CalculateHeaderLayout(80, 200, defaultLogoWidth)
	if layout.ShowLogo || layout.ContextWidth != 40 {
		t.Errorf("layout = %+v, want no logo and 40 columns of hints", layout)
	}
}

func TestCalculateHeaderLayout_zeroContextHelp(t *testing.T) {
	if layout := CalculateHeaderLayout(129, 0, defaultLogoWidth); layout.ContextWidth != 0 {
		t.Errorf("expected 0, got %d", layout.ContextWidth)
	}
}

func TestCalculateHeaderLayout_negativeContextHelp(t *testing.T) {
	if layout := CalculateHeaderLayout(200, -5, defaultLogoWidth); layout.ContextWidth != 0 {
		t.Errorf("expected 0 for negative input, got %d", layout.ContextWidth)
	}
}

func TestCalculateHeaderLayout_veryNarrowTerminal(t *testing.T) {
	if layout := CalculateHeaderLayout(40, 30, defaultLogoWidth); layout.ContextWidth != 0 || layout.ShowLogo {
		t.Errorf("layout = %+v, want nothing beside the info column", layout)
	}
}
