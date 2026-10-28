package app

import (
	_ "embed"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/theme"
)

//go:embed fonts/inter.ttf
var interFont []byte

//go:embed fonts/jetbrains-mono.ttf
var monoFont []byte

func registerFonts() error {
	if err := ui.RegisterFont(interFont, "Inter"); err != nil {
		return fmt.Errorf("register Inter font: %w", err)
	}
	if err := ui.RegisterFont(monoFont, "JetBrains Mono"); err != nil {
		return fmt.Errorf("register JetBrains Mono font: %w", err)
	}
	return nil
}

// designTheme applies the Pen palette at the shell, including native controls.
func designTheme(c *ui.Context) *ui.Theme {
	t := theme.Onevra(c)
	if t.Dark {
		t.Background = ui.Hex("#0B0D10")
		t.Surface = ui.Hex("#1C222A")
		t.SurfaceHover = ui.Hex("#252C36")
		t.SurfacePressed = ui.Hex("#343C49")
		t.Border = ui.Hex("#343C49")
		t.Text = ui.Hex("#E7EAEF")
		t.TextMuted = ui.Hex("#8C96A5")
		t.Accent = ui.Hex("#2F6FED")
		t.AccentHover = ui.Hex("#4580F5")
		t.Danger = ui.Hex("#F27379")
		t.Warning = ui.Hex("#E8A93F")
		t.Success = ui.Hex("#4CC391")
		t.Focus = ui.Hex("#4C8DFF")
	} else {
		t.Background = ui.Hex("#F4F6F8")
		t.Surface = ui.Hex("#EBEEF2")
		t.SurfaceHover = ui.Hex("#DFE3E9")
		t.SurfacePressed = ui.Hex("#BFC6D0")
		t.Border = ui.Hex("#BFC6D0")
		t.Text = ui.Hex("#0E1218")
		t.TextMuted = ui.Hex("#586272")
		t.Accent = ui.Hex("#1D5FE0")
		t.AccentHover = ui.Hex("#1750C4")
		t.Danger = ui.Hex("#B3262C")
		t.Warning = ui.Hex("#8F5E0F")
		t.Success = ui.Hex("#13784A")
		t.Focus = ui.Hex("#2F6FED")
	}
	t.AccentPressed = t.AccentHover.Mix(ui.RGB(0, 0, 0), .12)
	t.AccentText = ui.Hex("#FFFFFF")
	t.Selection = t.Accent.Alpha(.24)
	t.Scrollbar = t.TextMuted.Alpha(.5)
	t.Radius = 6
	t.Font = "Inter"
	t.FontSize = 13
	p := c.Preferences()
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, .45)
		t.TextMuted = t.TextMuted.Mix(t.Text, .4)
	}
	if p.TextScale > 0 {
		t.FontSize *= p.TextScale
	}
	return t
}

func panelBackground(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.Hex("#101318")
	}
	return ui.Hex("#FFFFFF")
}

func inputBackground(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.Hex("#0D1014")
	}
	return ui.Hex("#FFFFFF")
}

func sectionBorder(c *ui.Context) ui.Color {
	if c.Preferences().HighContrast {
		return c.Theme().Border
	}
	if c.Theme().Dark {
		return ui.Hex("#232933")
	}
	return ui.Hex("#DCE0E6")
}
