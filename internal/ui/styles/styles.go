package styles

import (
	"fmt"
	"image/color"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Icons used throughout the UI.
const (
	CheckIcon   = "✓"
	SpinnerIcon = "⋯"
	PendingIcon = "●"
	ErrorIcon   = "×"
	ModelIcon   = "◇"
	ArrowRight  = "→"
	BorderThin  = "│"
	BorderThick = "▌"
	Separator   = "─"
)

// Styles contains all visual styles for the application.
type Styles struct {
	// Base text styles
	Base      lipgloss.Style
	Muted     lipgloss.Style
	HalfMuted lipgloss.Style
	Subtle    lipgloss.Style
	Bold      lipgloss.Style

	// Tags
	TagError   lipgloss.Style
	TagInfo    lipgloss.Style
	TagWarning lipgloss.Style
	TagSuccess lipgloss.Style

	// Header
	Header struct {
		Model      lipgloss.Style
		Provider   lipgloss.Style
		WorkingDir lipgloss.Style
		Separator  lipgloss.Style
		Keystroke  lipgloss.Style
	}

	// Status bar
	StatusBar struct {
		Base     lipgloss.Style
		Key      lipgloss.Style
		Value    lipgloss.Style
		Accent   lipgloss.Style
		Divider  lipgloss.Style
		Provider lipgloss.Style
	}

	// Chat messages
	Chat struct {
		UserBorder      lipgloss.Style
		UserLabel       lipgloss.Style
		AssistantBorder lipgloss.Style
		AssistantLabel  lipgloss.Style
		Thinking        lipgloss.Style
		ThinkingFooter  lipgloss.Style
		ErrorTag        lipgloss.Style
		ErrorTitle      lipgloss.Style
		ErrorDetails    lipgloss.Style
	}

	// Tool calls
	Tool struct {
		IconPending   lipgloss.Style
		IconSuccess   lipgloss.Style
		IconError     lipgloss.Style
		NameNormal    lipgloss.Style
		ParamMain     lipgloss.Style
		ParamKey      lipgloss.Style
		Body          lipgloss.Style
		ContentLine   lipgloss.Style
		ContentCode   lipgloss.Style
		Truncation    lipgloss.Style
		StateWaiting  lipgloss.Style
		DiffAdd       lipgloss.Style
		DiffDel       lipgloss.Style
		DiffContext   lipgloss.Style
		DiffHeader    lipgloss.Style
	}

	// Input
	Input struct {
		Prompt  lipgloss.Style
		Cursor  lipgloss.Style
		Border  lipgloss.Style
		Focused lipgloss.Style
	}

	// Spinner
	SpinnerStyle lipgloss.Style

	// Markdown rendering
	Markdown ansi.StyleConfig

	// Semantic colors
	Primary   color.Color
	Secondary color.Color
	BgBase    color.Color
	BgSubtle  color.Color
	FgBase    color.Color
	FgMuted   color.Color
	Error     color.Color
	Warning   color.Color
	Success   color.Color
	Info      color.Color
	Green     color.Color
	Red       color.Color
	Yellow    color.Color
	Blue      color.Color
}

// New creates a Styles instance using the CharmTone palette.
func New(_ color.Color) *Styles {
	// CharmTone palette — matches crush's dark theme.
	var (
		primary   = charmtone.Charple
		secondary = charmtone.Dolly
		// tertiary  = charmtone.Bok

		bgBase   = charmtone.Pepper
		bgSubtle = charmtone.Charcoal

		fgBase    = charmtone.Ash
		fgMuted   = charmtone.Squid
		fgHalf    = charmtone.Smoke
		fgSubtle  = charmtone.Oyster
		border    = charmtone.Charcoal

		errColor = charmtone.Sriracha
		warn     = charmtone.Mustard
		info     = charmtone.Malibu

		white    = charmtone.Butter
		blue     = charmtone.Malibu
		green    = charmtone.Julep
		greenDk  = charmtone.Guac
		red      = charmtone.Coral
		yellow   = charmtone.Mustard
	)

	base := lipgloss.NewStyle().Foreground(fgBase)
	s := &Styles{}

	// Semantic colors.
	s.Primary = primary
	s.Secondary = secondary
	s.BgBase = bgBase
	s.BgSubtle = bgSubtle
	s.FgBase = fgBase
	s.FgMuted = fgMuted
	s.Error = errColor
	s.Warning = warn
	s.Success = greenDk
	s.Info = info
	s.Green = green
	s.Red = red
	s.Yellow = yellow
	s.Blue = blue

	// Base text styles.
	s.Base = base
	s.Muted = lipgloss.NewStyle().Foreground(fgMuted)
	s.HalfMuted = lipgloss.NewStyle().Foreground(fgHalf)
	s.Subtle = lipgloss.NewStyle().Foreground(fgSubtle)
	s.Bold = lipgloss.NewStyle().Foreground(fgBase).Bold(true)

	// Tags.
	s.TagError = lipgloss.NewStyle().Background(errColor).Foreground(white).Padding(0, 1).Bold(true)
	s.TagInfo = lipgloss.NewStyle().Background(info).Foreground(white).Padding(0, 1)
	s.TagWarning = lipgloss.NewStyle().Background(warn).Foreground(bgBase).Padding(0, 1)
	s.TagSuccess = lipgloss.NewStyle().Background(greenDk).Foreground(bgBase).Padding(0, 1)

	// Header.
	s.Header.Model = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Header.Provider = lipgloss.NewStyle().Foreground(fgMuted)
	s.Header.WorkingDir = lipgloss.NewStyle().Foreground(fgMuted)
	s.Header.Separator = lipgloss.NewStyle().Foreground(fgSubtle)
	s.Header.Keystroke = lipgloss.NewStyle().Foreground(fgMuted).Italic(true)

	// Status bar.
	s.StatusBar.Base = lipgloss.NewStyle().Background(bgSubtle).Foreground(fgBase)
	s.StatusBar.Key = lipgloss.NewStyle().Foreground(fgMuted)
	s.StatusBar.Value = lipgloss.NewStyle().Foreground(fgBase)
	s.StatusBar.Accent = lipgloss.NewStyle().Background(primary).Foreground(white).Padding(0, 1).Bold(true)
	s.StatusBar.Divider = lipgloss.NewStyle().Foreground(fgSubtle)
	s.StatusBar.Provider = lipgloss.NewStyle().Foreground(secondary)

	// Chat.
	s.Chat.UserBorder = lipgloss.NewStyle().Foreground(blue)
	s.Chat.UserLabel = lipgloss.NewStyle().Foreground(blue).Bold(true)
	s.Chat.AssistantBorder = lipgloss.NewStyle().Foreground(primary)
	s.Chat.AssistantLabel = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Chat.Thinking = lipgloss.NewStyle().Foreground(fgMuted).Italic(true).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(border).PaddingLeft(1)
	s.Chat.ThinkingFooter = lipgloss.NewStyle().Foreground(fgMuted).Italic(true)
	s.Chat.ErrorTag = lipgloss.NewStyle().Background(errColor).Foreground(white).Padding(0, 1).Bold(true)
	s.Chat.ErrorTitle = lipgloss.NewStyle().Foreground(errColor).Bold(true)
	s.Chat.ErrorDetails = lipgloss.NewStyle().Foreground(fgMuted)

	// Tool calls.
	s.Tool.IconPending = lipgloss.NewStyle().Foreground(yellow)
	s.Tool.IconSuccess = lipgloss.NewStyle().Foreground(green)
	s.Tool.IconError = lipgloss.NewStyle().Foreground(red)
	s.Tool.NameNormal = lipgloss.NewStyle().Foreground(fgBase).Bold(true)
	s.Tool.ParamMain = lipgloss.NewStyle().Foreground(fgBase)
	s.Tool.ParamKey = lipgloss.NewStyle().Foreground(fgMuted)
	s.Tool.Body = lipgloss.NewStyle().PaddingLeft(2)
	s.Tool.ContentLine = lipgloss.NewStyle().Background(bgSubtle).Foreground(fgBase)
	s.Tool.ContentCode = lipgloss.NewStyle().Background(bgSubtle).Foreground(fgBase)
	s.Tool.Truncation = lipgloss.NewStyle().Foreground(fgMuted).Italic(true)
	s.Tool.StateWaiting = lipgloss.NewStyle().Foreground(fgMuted).Italic(true)
	s.Tool.DiffAdd = lipgloss.NewStyle().Foreground(green)
	s.Tool.DiffDel = lipgloss.NewStyle().Foreground(red)
	s.Tool.DiffContext = lipgloss.NewStyle().Foreground(fgMuted)
	s.Tool.DiffHeader = lipgloss.NewStyle().Foreground(blue).Bold(true)

	// Input.
	s.Input.Prompt = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Input.Cursor = lipgloss.NewStyle().Foreground(primary)
	s.Input.Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)
	s.Input.Focused = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(0, 1)

	// Spinner.
	s.SpinnerStyle = lipgloss.NewStyle().Foreground(primary)

	// Markdown.
	s.Markdown = markdownStyle()

	return s
}

// markdownStyle creates a glamour-compatible style config.
func markdownStyle() ansi.StyleConfig {
	primary := charmtone.Charple.Hex()
	info := charmtone.Malibu.Hex()
	muted := charmtone.Squid.Hex()
	bg := charmtone.Charcoal.Hex()

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
			},
			Margin: uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: &primary,
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:   boolPtr(true),
				Color:  &primary,
				Prefix: "# ",
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:   boolPtr(true),
				Color:  &primary,
				Prefix: "## ",
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:   boolPtr(true),
				Color:  &primary,
				Prefix: "### ",
			},
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:           &info,
				BackgroundColor: &bg,
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				Margin: uintPtr(1),
			},
			Chroma: &ansi.Chroma{},
		},
		Link: ansi.StylePrimitive{
			Color:     &info,
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		List: ansi.StyleList{
			StyleBlock:  ansi.StyleBlock{},
			LevelIndent: 2,
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "  ",
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{},
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  &muted,
				Italic: boolPtr(true),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Table: ansi.StyleTable{
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
	}
}

func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }
func boolPtr(b bool) *bool       { return &b }

// Suppress unused import warning for fmt.
var _ = fmt.Sprint
