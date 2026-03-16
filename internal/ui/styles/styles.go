package styles

import (
	"image/color"
	"os"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Icons used throughout the UI.
const (
	CheckIcon      = "✓"
	SpinnerIcon    = "⋯"
	PendingIcon    = "●"
	ErrorIcon      = "×"
	ModelIcon      = "◇"
	ArrowRight     = "→"
	BorderThin     = "│"
	BorderThick    = "▌"
	Separator      = "─"
	HollowIcon     = "○"
	InProgressIcon = "◐"
	BlockedIcon    = "✗"
	PromptIcon     = "❯"
	ResultPrefix   = "⎿"
)

// Styles contains all visual styles for the application.
type Styles struct {
	// Base text styles.
	Base       lipgloss.Style
	Muted      lipgloss.Style
	HalfMuted  lipgloss.Style
	Subtle     lipgloss.Style
	Bold       lipgloss.Style
	Emphasis   lipgloss.Style
	Meta       lipgloss.Style
	Border     lipgloss.Style
	Surface    lipgloss.Style
	SurfaceAlt lipgloss.Style
	InlineCode lipgloss.Style
	CodeBlock  lipgloss.Style

	// Tags.
	TagError   lipgloss.Style
	TagInfo    lipgloss.Style
	TagWarning lipgloss.Style
	TagSuccess lipgloss.Style

	// Header.
	Header struct {
		Model      lipgloss.Style
		Provider   lipgloss.Style
		WorkingDir lipgloss.Style
		Separator  lipgloss.Style
		Keystroke  lipgloss.Style
	}

	// Shell framing.
	Shell struct {
		SectionLabel lipgloss.Style
		SectionMeta  lipgloss.Style
		Rule         lipgloss.Style
	}

	// Status bar.
	StatusBar struct {
		Base     lipgloss.Style
		Key      lipgloss.Style
		Value    lipgloss.Style
		Accent   lipgloss.Style
		Divider  lipgloss.Style
		Provider lipgloss.Style
	}

	// Chat messages.
	Chat struct {
		UserBorder      lipgloss.Style
		UserLabel       lipgloss.Style
		AssistantBorder lipgloss.Style
		AssistantLabel  lipgloss.Style
		Thinking        lipgloss.Style
		ThinkingFooter  lipgloss.Style
		Streaming       lipgloss.Style
		Running         lipgloss.Style
		Summary         lipgloss.Style
		AssistantMeta   lipgloss.Style
		UserTag         lipgloss.Style
		AssistantTag    lipgloss.Style
		ThinkingTag     lipgloss.Style
		SystemTag       lipgloss.Style
		SystemText      lipgloss.Style
		ErrorTag        lipgloss.Style
		ErrorTitle      lipgloss.Style
		ErrorDetails    lipgloss.Style
	}

	// Tool calls.
	Tool struct {
		IconPending   lipgloss.Style
		IconSuccess   lipgloss.Style
		IconError     lipgloss.Style
		NameNormal    lipgloss.Style
		ParamMain     lipgloss.Style
		ParamKey      lipgloss.Style
		CommandPrompt lipgloss.Style
		CommandText   lipgloss.Style
		Body          lipgloss.Style
		ContentLine   lipgloss.Style
		ContentCode   lipgloss.Style
		OutputBorder  lipgloss.Style
		ResultPrefix  lipgloss.Style
		OutputMeta    lipgloss.Style
		Truncation    lipgloss.Style
		StateWaiting  lipgloss.Style
		StateRunning  lipgloss.Style
		StateSuccess  lipgloss.Style
		StateError    lipgloss.Style
		Summary       lipgloss.Style
		DiffAdd       lipgloss.Style
		DiffDel       lipgloss.Style
		DiffContext   lipgloss.Style
		DiffHeader    lipgloss.Style
	}

	// Input.
	Input struct {
		Prompt  lipgloss.Style
		Cursor  lipgloss.Style
		Border  lipgloss.Style
		Focused lipgloss.Style
	}

	// Right-hand panel.
	Panel struct {
		Base             lipgloss.Style
		Title            lipgloss.Style
		Separator        lipgloss.Style
		Progress         lipgloss.Style
		TaskText         lipgloss.Style
		TaskDone         lipgloss.Style
		IconPending      lipgloss.Style
		IconInProgress   lipgloss.Style
		IconCompleted    lipgloss.Style
		IconBlocked      lipgloss.Style
		HeaderActive     lipgloss.Style
		HeaderInactive   lipgloss.Style
		HeaderMeta       lipgloss.Style
		HeaderKey        lipgloss.Style
		EmptyTitle       lipgloss.Style
		EmptyBody        lipgloss.Style
		EmptyHint        lipgloss.Style
		FocusTabActive   lipgloss.Style
		FocusTabInactive lipgloss.Style
		MetricKey        lipgloss.Style
		MetricValue      lipgloss.Style
	}

	// Spinner.
	SpinnerStyle lipgloss.Style

	// Markdown rendering.
	Markdown ansi.StyleConfig

	// Semantic colors.
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
	IsDark    bool
	NoColor   bool
}

// New creates a Styles instance using terminal background information when available.
func New(bg color.Color) *Styles {
	return NewMode(bg, deriveMode(bg))
}

// NewMode creates a Styles instance using an explicit dark/light mode override.
// If mode is nil, terminal background information is used when available.
func NewMode(bg color.Color, mode *bool) *Styles {
	noColor := noColorEnabled()
	isDark := true
	if mode != nil {
		isDark = *mode
	} else if bg != nil {
		isDark = isDarkColor(bg)
	}

	palette := adaptivePalette(isDark, noColor)

	base := lipgloss.NewStyle().Foreground(palette.fgBase)
	meta := lipgloss.NewStyle().Foreground(palette.fgMuted)
	halfMuted := lipgloss.NewStyle().Foreground(palette.fgHalf)
	subtle := lipgloss.NewStyle().Foreground(palette.fgSubtle)
	borderStyle := lipgloss.NewStyle().Foreground(palette.border)
	strong := lipgloss.NewStyle().Foreground(palette.fgStrong).Bold(true)

	s := &Styles{IsDark: isDark, NoColor: noColor}

	// Semantic colors.
	s.Primary = palette.primary
	s.Secondary = palette.secondary
	s.BgBase = palette.bgBase
	s.BgSubtle = palette.bgSubtle
	s.FgBase = palette.fgBase
	s.FgMuted = palette.fgMuted
	s.Error = palette.errColor
	s.Warning = palette.warn
	s.Success = palette.success
	s.Info = palette.info
	s.Green = palette.green
	s.Red = palette.red
	s.Yellow = palette.yellow
	s.Blue = palette.blue

	// Base text styles.
	s.Base = base
	s.Muted = meta
	s.HalfMuted = halfMuted
	s.Subtle = subtle
	s.Bold = strong
	s.Emphasis = lipgloss.NewStyle().Foreground(palette.primary).Bold(true)
	s.Meta = meta
	s.Border = borderStyle
	s.Surface = lipgloss.NewStyle().Background(palette.surface).Foreground(palette.fgBase)
	s.SurfaceAlt = lipgloss.NewStyle().Background(palette.bgBase).Foreground(palette.fgBase)
	s.InlineCode = lipgloss.NewStyle().Foreground(palette.info).Background(palette.surface).Padding(0, 1)
	s.CodeBlock = lipgloss.NewStyle().Foreground(palette.fgBase).Background(palette.surface).Padding(0, 1)

	// Tags.
	s.TagError = tagStyle(palette.errColor, palette.tagOnError, noColor)
	s.TagInfo = tagStyle(palette.info, palette.tagOnInfo, noColor)
	s.TagWarning = tagStyle(palette.warn, palette.tagOnWarning, noColor)
	s.TagSuccess = tagStyle(palette.success, palette.tagOnSuccess, noColor)

	// Header.
	s.Header.Model = lipgloss.NewStyle().Foreground(palette.primary).Bold(true)
	s.Header.Provider = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)
	s.Header.WorkingDir = halfMuted.Italic(true)
	s.Header.Separator = subtle
	s.Header.Keystroke = meta.Italic(true)

	// Shell framing.
	s.Shell.SectionLabel = lipgloss.NewStyle().Foreground(palette.fgStrong).Background(palette.surface).Padding(0, 1).Bold(true)
	s.Shell.SectionMeta = lipgloss.NewStyle().Foreground(palette.fgHalf)
	s.Shell.Rule = lipgloss.NewStyle().Foreground(palette.border)

	// Status bar.
	s.StatusBar.Base = lipgloss.NewStyle().Background(palette.bgSubtle).Foreground(palette.fgBase)
	s.StatusBar.Key = halfMuted
	s.StatusBar.Value = lipgloss.NewStyle().Foreground(palette.fgStrong)
	s.StatusBar.Accent = lipgloss.NewStyle().Background(palette.primary).Foreground(palette.accentText).Padding(0, 1).Bold(true)
	s.StatusBar.Divider = subtle
	s.StatusBar.Provider = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)

	// Chat.
	s.Chat.UserBorder = lipgloss.NewStyle().Foreground(palette.blue)
	s.Chat.UserLabel = lipgloss.NewStyle().Foreground(palette.blue).Bold(true)
	s.Chat.AssistantBorder = lipgloss.NewStyle().Foreground(palette.primary)
	s.Chat.AssistantLabel = lipgloss.NewStyle().Foreground(palette.primary).Bold(true)
	s.Chat.Thinking = lipgloss.NewStyle().
		Foreground(palette.fgHalf).
		Background(palette.surface).
		Italic(true).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(palette.border).
		PaddingLeft(1)
	s.Chat.ThinkingFooter = meta.Italic(true)
	s.Chat.Streaming = lipgloss.NewStyle().Foreground(palette.accentText).Background(palette.primary).Padding(0, 1).Bold(true)
	s.Chat.Running = lipgloss.NewStyle().Foreground(palette.yellow).Bold(true)
	s.Chat.Summary = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)
	s.Chat.AssistantMeta = meta.Italic(true)
	s.Chat.UserTag = tagStyle(palette.blue, palette.tagOnBlue, noColor)
	s.Chat.AssistantTag = tagStyle(palette.primary, palette.tagOnPrimary, noColor)
	s.Chat.ThinkingTag = tagStyle(palette.surface, palette.fgStrong, noColor)
	s.Chat.SystemTag = tagStyle(palette.secondary, palette.tagOnSecondary, noColor)
	s.Chat.SystemText = meta
	s.Chat.ErrorTag = tagStyle(palette.errColor, palette.tagOnError, noColor)
	s.Chat.ErrorTitle = lipgloss.NewStyle().Foreground(palette.red).Bold(true)
	s.Chat.ErrorDetails = meta

	// Tool calls.
	s.Tool.IconPending = lipgloss.NewStyle().Foreground(palette.yellow).Bold(true)
	s.Tool.IconSuccess = lipgloss.NewStyle().Foreground(palette.green).Bold(true)
	s.Tool.IconError = lipgloss.NewStyle().Foreground(palette.red).Bold(true)
	s.Tool.NameNormal = lipgloss.NewStyle().Foreground(palette.fgStrong).Bold(true)
	s.Tool.ParamMain = halfMuted
	s.Tool.ParamKey = meta
	s.Tool.CommandPrompt = lipgloss.NewStyle().Foreground(palette.success).Bold(true)
	s.Tool.CommandText = lipgloss.NewStyle().Foreground(palette.fgStrong)
	s.Tool.Body = lipgloss.NewStyle().PaddingLeft(2)
	s.Tool.ContentLine = halfMuted
	s.Tool.ContentCode = lipgloss.NewStyle().Foreground(palette.info)
	s.Tool.OutputBorder = borderStyle
	s.Tool.ResultPrefix = meta
	s.Tool.OutputMeta = meta.Italic(true)
	s.Tool.Truncation = subtle.Italic(true)
	s.Tool.StateWaiting = meta.Italic(true)
	s.Tool.StateRunning = lipgloss.NewStyle().Foreground(palette.yellow).Italic(true)
	s.Tool.StateSuccess = lipgloss.NewStyle().Foreground(palette.success).Italic(true)
	s.Tool.StateError = lipgloss.NewStyle().Foreground(palette.red).Italic(true)
	s.Tool.Summary = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)
	s.Tool.DiffAdd = lipgloss.NewStyle().Foreground(palette.green)
	s.Tool.DiffDel = lipgloss.NewStyle().Foreground(palette.red)
	s.Tool.DiffContext = meta
	s.Tool.DiffHeader = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)

	// Input.
	s.Input.Prompt = lipgloss.NewStyle().Foreground(palette.primary).Bold(true)
	s.Input.Cursor = lipgloss.NewStyle().Foreground(palette.primary)
	s.Input.Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(palette.border).
		Padding(0, 1)
	s.Input.Focused = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(palette.primary).
		Padding(0, 1)

	// Panel.
	s.Panel.Base = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(palette.border).
		Background(palette.bgSubtle).
		PaddingLeft(1)
	s.Panel.Title = lipgloss.NewStyle().Foreground(palette.primary).Bold(true)
	s.Panel.Separator = lipgloss.NewStyle().Foreground(palette.border)
	s.Panel.Progress = lipgloss.NewStyle().Foreground(palette.secondary).Bold(true)
	s.Panel.TaskText = lipgloss.NewStyle().Foreground(palette.fgStrong)
	s.Panel.TaskDone = lipgloss.NewStyle().Foreground(palette.fgMuted).Strikethrough(true)
	s.Panel.IconPending = meta
	s.Panel.IconInProgress = lipgloss.NewStyle().Foreground(palette.yellow).Bold(true)
	s.Panel.IconCompleted = lipgloss.NewStyle().Foreground(palette.green).Bold(true)
	s.Panel.IconBlocked = lipgloss.NewStyle().Foreground(palette.red).Bold(true)
	s.Panel.HeaderActive = lipgloss.NewStyle().Foreground(palette.accentText).Background(palette.primary).Padding(0, 1).Bold(true)
	s.Panel.HeaderInactive = lipgloss.NewStyle().Foreground(palette.fgStrong).Background(palette.bgSubtle).Padding(0, 1).Bold(true)
	s.Panel.HeaderMeta = lipgloss.NewStyle().Foreground(palette.fgHalf)
	s.Panel.HeaderKey = lipgloss.NewStyle().Foreground(palette.blue).Bold(true)
	s.Panel.EmptyTitle = lipgloss.NewStyle().Foreground(palette.fgStrong).Background(palette.bgSubtle).Padding(0, 1).Bold(true)
	s.Panel.EmptyBody = lipgloss.NewStyle().Foreground(palette.fgMuted).Background(palette.surface).Padding(0, 1)
	s.Panel.EmptyHint = lipgloss.NewStyle().Foreground(palette.info).Background(palette.surface).Padding(0, 1).Bold(true)
	s.Panel.FocusTabActive = lipgloss.NewStyle().Foreground(palette.accentText).Background(palette.primary).Padding(0, 1).Bold(true)
	s.Panel.FocusTabInactive = lipgloss.NewStyle().Foreground(palette.fgMuted).Background(palette.surface).Padding(0, 1)
	s.Panel.MetricKey = lipgloss.NewStyle().Foreground(palette.fgHalf)
	s.Panel.MetricValue = lipgloss.NewStyle().Foreground(palette.fgStrong).Bold(true)

	// Spinner.
	s.SpinnerStyle = lipgloss.NewStyle().Foreground(palette.primary)

	// Markdown.
	s.Markdown = markdownStyle(palette)

	return s
}

func deriveMode(bg color.Color) *bool {
	if bg == nil {
		return nil
	}
	isDark := isDarkColor(bg)
	return &isDark
}

func noColorEnabled() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

func isDarkColor(c color.Color) bool {
	if c == nil {
		return true
	}
	if _, ok := c.(lipgloss.NoColor); ok {
		return true
	}
	r, g, b, _ := c.RGBA()
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
	return luminance < 0.5
}

type themePalette struct {
	primary        color.Color
	secondary      color.Color
	bgBase         color.Color
	bgSubtle       color.Color
	fgBase         color.Color
	fgStrong       color.Color
	fgMuted        color.Color
	fgHalf         color.Color
	fgSubtle       color.Color
	border         color.Color
	surface        color.Color
	errColor       color.Color
	warn           color.Color
	info           color.Color
	success        color.Color
	green          color.Color
	red            color.Color
	yellow         color.Color
	blue           color.Color
	accentText     color.Color
	tagOnError     color.Color
	tagOnInfo      color.Color
	tagOnWarning   color.Color
	tagOnSuccess   color.Color
	tagOnBlue      color.Color
	tagOnPrimary   color.Color
	tagOnSecondary color.Color

	primaryHex   string
	secondaryHex string
	accentHex    string
	successHex   string
	warningHex   string
	errorHex     string
	strongHex    string
	fgBaseHex    string
	fgMutedHex   string
	fgSoftHex    string
	bgSurfaceHex string
	borderHex    string
}

func adaptivePalette(isDark, noColor bool) themePalette {
	var palette themePalette
	if isDark {
		palette = darkPalette()
	} else {
		palette = lightPalette()
	}
	if noColor {
		palette = withoutColor(palette)
	}
	palette.accentText = readableText(palette.primary)
	palette.tagOnError = readableText(palette.errColor)
	palette.tagOnInfo = readableText(palette.info)
	palette.tagOnWarning = readableText(palette.warn)
	palette.tagOnSuccess = readableText(palette.success)
	palette.tagOnBlue = readableText(palette.blue)
	palette.tagOnPrimary = readableText(palette.primary)
	palette.tagOnSecondary = readableText(palette.secondary)
	return palette
}

func darkPalette() themePalette {
	return themePalette{
		primary:      charmtone.Charple,
		secondary:    charmtone.Dolly,
		bgBase:       charmtone.Pepper,
		bgSubtle:     charmtone.Charcoal,
		fgBase:       charmtone.Ash,
		fgStrong:     charmtone.Butter,
		fgMuted:      charmtone.Squid,
		fgHalf:       charmtone.Smoke,
		fgSubtle:     charmtone.Oyster,
		border:       charmtone.Squid,
		surface:      charmtone.Charcoal,
		errColor:     charmtone.Sriracha,
		warn:         charmtone.Mustard,
		info:         charmtone.Malibu,
		success:      charmtone.Guac,
		green:        charmtone.Julep,
		red:          charmtone.Coral,
		yellow:       charmtone.Mustard,
		blue:         charmtone.Malibu,
		primaryHex:   charmtone.Charple.Hex(),
		secondaryHex: charmtone.Dolly.Hex(),
		accentHex:    charmtone.Malibu.Hex(),
		successHex:   charmtone.Julep.Hex(),
		warningHex:   charmtone.Mustard.Hex(),
		errorHex:     charmtone.Coral.Hex(),
		strongHex:    charmtone.Butter.Hex(),
		fgBaseHex:    charmtone.Ash.Hex(),
		fgMutedHex:   charmtone.Squid.Hex(),
		fgSoftHex:    charmtone.Smoke.Hex(),
		bgSurfaceHex: charmtone.Charcoal.Hex(),
		borderHex:    charmtone.Oyster.Hex(),
	}
}

func lightPalette() themePalette {
	return themePalette{
		primary:      lipgloss.Color("#7B61FF"),
		secondary:    lipgloss.Color("#B45FE0"),
		bgBase:       lipgloss.Color("#FFFDF5"),
		bgSubtle:     lipgloss.Color("#F4EFE4"),
		fgBase:       lipgloss.Color("#1F2430"),
		fgStrong:     lipgloss.Color("#12161F"),
		fgMuted:      lipgloss.Color("#5E6675"),
		fgHalf:       lipgloss.Color("#768091"),
		fgSubtle:     lipgloss.Color("#99A2AF"),
		border:       lipgloss.Color("#D8D3C7"),
		surface:      lipgloss.Color("#F7F2E8"),
		errColor:     lipgloss.Color("#C3414A"),
		warn:         lipgloss.Color("#9A6A00"),
		info:         lipgloss.Color("#0B82A8"),
		success:      lipgloss.Color("#1F7A4F"),
		green:        lipgloss.Color("#2B9B67"),
		red:          lipgloss.Color("#C3414A"),
		yellow:       lipgloss.Color("#A97100"),
		blue:         lipgloss.Color("#2E6DD8"),
		primaryHex:   "#7B61FF",
		secondaryHex: "#B45FE0",
		accentHex:    "#0B82A8",
		successHex:   "#2B9B67",
		warningHex:   "#A97100",
		errorHex:     "#C3414A",
		strongHex:    "#12161F",
		fgBaseHex:    "#1F2430",
		fgMutedHex:   "#5E6675",
		fgSoftHex:    "#768091",
		bgSurfaceHex: "#F7F2E8",
		borderHex:    "#D8D3C7",
	}
}

func withoutColor(p themePalette) themePalette {
	noColor := lipgloss.NoColor{}
	p.primary = noColor
	p.secondary = noColor
	p.bgBase = noColor
	p.bgSubtle = noColor
	p.fgBase = noColor
	p.fgStrong = noColor
	p.fgMuted = noColor
	p.fgHalf = noColor
	p.fgSubtle = noColor
	p.border = noColor
	p.surface = noColor
	p.errColor = noColor
	p.warn = noColor
	p.info = noColor
	p.success = noColor
	p.green = noColor
	p.red = noColor
	p.yellow = noColor
	p.blue = noColor
	p.accentText = noColor
	p.tagOnError = noColor
	p.tagOnInfo = noColor
	p.tagOnWarning = noColor
	p.tagOnSuccess = noColor
	p.tagOnBlue = noColor
	p.tagOnPrimary = noColor
	p.tagOnSecondary = noColor
	p.primaryHex = ""
	p.secondaryHex = ""
	p.accentHex = ""
	p.successHex = ""
	p.warningHex = ""
	p.errorHex = ""
	p.strongHex = ""
	p.fgBaseHex = ""
	p.fgMutedHex = ""
	p.fgSoftHex = ""
	p.bgSurfaceHex = ""
	p.borderHex = ""
	return p
}

func readableText(bg color.Color) color.Color {
	if bg == nil {
		return lipgloss.NoColor{}
	}
	if _, ok := bg.(lipgloss.NoColor); ok {
		return lipgloss.NoColor{}
	}
	if isDarkColor(bg) {
		return charmtone.Butter
	}
	return lipgloss.Color("#111318")
}

func tagStyle(bg, fg color.Color, noColor bool) lipgloss.Style {
	style := lipgloss.NewStyle().Padding(0, 1).Bold(true)
	if noColor {
		return style
	}
	return style.Background(bg).Foreground(fg)
}

// markdownStyle creates a glamour-compatible style config.
func markdownStyle(palette themePalette) ansi.StyleConfig {
	primary := colorStringPtr(palette.primaryHex)
	secondary := colorStringPtr(palette.secondaryHex)
	accent := colorStringPtr(palette.accentHex)
	success := colorStringPtr(palette.successHex)
	warning := colorStringPtr(palette.warningHex)
	errorColor := colorStringPtr(palette.errorHex)
	strong := colorStringPtr(palette.strongHex)
	fgBase := colorStringPtr(palette.fgBaseHex)
	fgMuted := colorStringPtr(palette.fgMutedHex)
	fgSoft := colorStringPtr(palette.fgSoftHex)
	bgSurface := colorStringPtr(palette.bgSurfaceHex)
	border := colorStringPtr(palette.borderHex)

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:       fgBase,
				BlockSuffix: "\n",
			},
			Margin: uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       strong,
				BlockSuffix: "\n",
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       primary,
				Prefix:      "# ",
				BlockSuffix: "\n",
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       strong,
				Prefix:      "## ",
				BlockSuffix: "\n",
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       secondary,
				Prefix:      "### ",
				BlockSuffix: "\n",
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       accent,
				Prefix:      "#### ",
				BlockSuffix: "\n",
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       fgBase,
				Prefix:      "##### ",
				BlockSuffix: "\n",
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       fgSoft,
				Prefix:      "###### ",
				BlockSuffix: "\n",
			},
		},
		Text: ansi.StylePrimitive{Color: fgBase},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:           accent,
				BackgroundColor: bgSurface,
				Prefix:          " ",
				Suffix:          " ",
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n"},
				Margin:         uintPtr(1),
			},
			Chroma: &ansi.Chroma{
				Background: ansi.StylePrimitive{BackgroundColor: bgSurface},
				Text:       ansi.StylePrimitive{Color: fgBase},
				Comment: ansi.StylePrimitive{
					Color:  fgMuted,
					Italic: boolPtr(true),
				},
				CommentPreproc:      ansi.StylePrimitive{Color: secondary},
				Keyword:             ansi.StylePrimitive{Color: primary, Bold: boolPtr(true)},
				KeywordReserved:     ansi.StylePrimitive{Color: primary, Bold: boolPtr(true)},
				KeywordNamespace:    ansi.StylePrimitive{Color: secondary},
				KeywordType:         ansi.StylePrimitive{Color: secondary, Bold: boolPtr(true)},
				Operator:            ansi.StylePrimitive{Color: fgSoft},
				Punctuation:         ansi.StylePrimitive{Color: fgMuted},
				Name:                ansi.StylePrimitive{Color: fgBase},
				NameBuiltin:         ansi.StylePrimitive{Color: accent},
				NameTag:             ansi.StylePrimitive{Color: primary, Bold: boolPtr(true)},
				NameAttribute:       ansi.StylePrimitive{Color: secondary},
				NameClass:           ansi.StylePrimitive{Color: accent, Bold: boolPtr(true)},
				NameConstant:        ansi.StylePrimitive{Color: warning},
				NameDecorator:       ansi.StylePrimitive{Color: primary},
				NameException:       ansi.StylePrimitive{Color: errorColor},
				NameFunction:        ansi.StylePrimitive{Color: accent, Bold: boolPtr(true)},
				NameOther:           ansi.StylePrimitive{Color: fgBase},
				Literal:             ansi.StylePrimitive{Color: success},
				LiteralNumber:       ansi.StylePrimitive{Color: warning},
				LiteralDate:         ansi.StylePrimitive{Color: secondary},
				LiteralString:       ansi.StylePrimitive{Color: success},
				LiteralStringEscape: ansi.StylePrimitive{Color: warning},
				GenericDeleted:      ansi.StylePrimitive{Color: errorColor},
				GenericEmph:         ansi.StylePrimitive{Italic: boolPtr(true)},
				GenericInserted:     ansi.StylePrimitive{Color: success},
				GenericStrong:       ansi.StylePrimitive{Bold: boolPtr(true)},
				GenericSubheading:   ansi.StylePrimitive{Color: primary, Bold: boolPtr(true)},
			},
		},
		Link: ansi.StylePrimitive{
			Color:     accent,
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{Color: accent, Bold: boolPtr(true)},
		Image:    ansi.StylePrimitive{Color: secondary},
		ImageText: ansi.StylePrimitive{
			Color: accent,
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  secondary,
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{Bold: boolPtr(true), Color: strong},
		HorizontalRule: ansi.StylePrimitive{
			Color:  border,
			Format: "\n────────\n",
		},
		List: ansi.StyleList{
			StyleBlock:  ansi.StyleBlock{},
			LevelIndent: 2,
		},
		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{Color: fgBase},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Paragraph: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: fgBase}},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
			Faint:      boolPtr(true),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: fgSoft, Italic: boolPtr(true)},
			Indent:         uintPtr(1),
			IndentToken:    stringPtr("│ "),
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: fgBase, BlockSuffix: "\n"},
			},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionList: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n"}},
		DefinitionTerm: ansi.StylePrimitive{Color: secondary, Bold: boolPtr(true)},
		DefinitionDescription: ansi.StylePrimitive{
			Color:       fgSoft,
			BlockPrefix: "  ",
		},
		HTMLBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: fgSoft}},
		HTMLSpan:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: fgSoft}},
	}
}

func colorStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }
func boolPtr(b bool) *bool       { return &b }
