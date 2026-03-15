package styles

import (
	"image/color"

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
		Base           lipgloss.Style
		Title          lipgloss.Style
		Separator      lipgloss.Style
		Progress       lipgloss.Style
		TaskText       lipgloss.Style
		TaskDone       lipgloss.Style
		IconPending    lipgloss.Style
		IconInProgress lipgloss.Style
		IconCompleted  lipgloss.Style
		IconBlocked    lipgloss.Style
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
}

// New creates a Styles instance using the CharmTone palette.
func New(_ color.Color) *Styles {
	var (
		primary   = charmtone.Charple
		secondary = charmtone.Dolly

		bgBase   = charmtone.Pepper
		bgSubtle = charmtone.Charcoal
		fgBase   = charmtone.Ash
		fgStrong = charmtone.Butter
		fgMuted  = charmtone.Squid
		fgHalf   = charmtone.Smoke
		fgSubtle = charmtone.Oyster
		border   = charmtone.Squid
		surface  = charmtone.Charcoal
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
	meta := lipgloss.NewStyle().Foreground(fgMuted)
	halfMuted := lipgloss.NewStyle().Foreground(fgHalf)
	subtle := lipgloss.NewStyle().Foreground(fgSubtle)
	borderStyle := lipgloss.NewStyle().Foreground(border)
	strong := lipgloss.NewStyle().Foreground(fgStrong).Bold(true)

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
	s.Muted = meta
	s.HalfMuted = halfMuted
	s.Subtle = subtle
	s.Bold = strong
	s.Emphasis = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Meta = meta
	s.Border = borderStyle
	s.Surface = lipgloss.NewStyle().Background(surface).Foreground(fgBase)
	s.SurfaceAlt = lipgloss.NewStyle().Background(bgBase).Foreground(fgBase)
	s.InlineCode = lipgloss.NewStyle().Foreground(info).Background(surface).Padding(0, 1)
	s.CodeBlock = lipgloss.NewStyle().Foreground(fgBase).Background(surface).Padding(0, 1)

	// Tags.
	s.TagError = tagStyle(errColor, white)
	s.TagInfo = tagStyle(info, white)
	s.TagWarning = tagStyle(warn, bgBase)
	s.TagSuccess = tagStyle(greenDk, bgBase)

	// Header.
	s.Header.Model = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Header.Provider = lipgloss.NewStyle().Foreground(secondary).Bold(true)
	s.Header.WorkingDir = halfMuted
	s.Header.Separator = subtle
	s.Header.Keystroke = meta.Italic(true)

	// Status bar.
	s.StatusBar.Base = lipgloss.NewStyle().Background(bgSubtle).Foreground(fgBase)
	s.StatusBar.Key = halfMuted
	s.StatusBar.Value = lipgloss.NewStyle().Foreground(fgStrong)
	s.StatusBar.Accent = lipgloss.NewStyle().Background(primary).Foreground(bgBase).Padding(0, 1).Bold(true)
	s.StatusBar.Divider = subtle
	s.StatusBar.Provider = lipgloss.NewStyle().Foreground(secondary).Bold(true)

	// Chat.
	s.Chat.UserBorder = lipgloss.NewStyle().Foreground(blue)
	s.Chat.UserLabel = lipgloss.NewStyle().Foreground(blue).Bold(true)
	s.Chat.AssistantBorder = lipgloss.NewStyle().Foreground(primary)
	s.Chat.AssistantLabel = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Chat.Thinking = lipgloss.NewStyle().
		Foreground(fgHalf).
		Background(surface).
		Italic(true).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(border).
		PaddingLeft(1)
	s.Chat.ThinkingFooter = meta.Italic(true)
	s.Chat.Streaming = lipgloss.NewStyle().Foreground(bgBase).Background(primary).Padding(0, 1).Bold(true)
	s.Chat.Running = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	s.Chat.Summary = lipgloss.NewStyle().Foreground(secondary).Bold(true)
	s.Chat.AssistantMeta = meta.Italic(true)
	s.Chat.UserTag = tagStyle(blue, bgBase)
	s.Chat.AssistantTag = tagStyle(primary, bgBase)
	s.Chat.ThinkingTag = tagStyle(surface, fgStrong)
	s.Chat.SystemTag = tagStyle(secondary, bgBase)
	s.Chat.SystemText = meta
	s.Chat.ErrorTag = tagStyle(errColor, white)
	s.Chat.ErrorTitle = lipgloss.NewStyle().Foreground(red).Bold(true)
	s.Chat.ErrorDetails = meta

	// Tool calls.
	s.Tool.IconPending = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	s.Tool.IconSuccess = lipgloss.NewStyle().Foreground(green).Bold(true)
	s.Tool.IconError = lipgloss.NewStyle().Foreground(red).Bold(true)
	s.Tool.NameNormal = lipgloss.NewStyle().Foreground(fgStrong).Bold(true)
	s.Tool.ParamMain = halfMuted
	s.Tool.ParamKey = meta
	s.Tool.CommandPrompt = lipgloss.NewStyle().Foreground(greenDk).Bold(true)
	s.Tool.CommandText = lipgloss.NewStyle().Foreground(fgStrong)
	s.Tool.Body = lipgloss.NewStyle().PaddingLeft(2)
	s.Tool.ContentLine = halfMuted
	s.Tool.ContentCode = lipgloss.NewStyle().Foreground(info)
	s.Tool.OutputBorder = borderStyle
	s.Tool.ResultPrefix = meta
	s.Tool.OutputMeta = meta.Italic(true)
	s.Tool.Truncation = subtle.Italic(true)
	s.Tool.StateWaiting = meta.Italic(true)
	s.Tool.StateRunning = lipgloss.NewStyle().Foreground(yellow).Italic(true)
	s.Tool.StateSuccess = lipgloss.NewStyle().Foreground(greenDk).Italic(true)
	s.Tool.StateError = lipgloss.NewStyle().Foreground(red).Italic(true)
	s.Tool.Summary = lipgloss.NewStyle().Foreground(secondary).Bold(true)
	s.Tool.DiffAdd = lipgloss.NewStyle().Foreground(green)
	s.Tool.DiffDel = lipgloss.NewStyle().Foreground(red)
	s.Tool.DiffContext = meta
	s.Tool.DiffHeader = lipgloss.NewStyle().Foreground(secondary).Bold(true)

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

	// Panel.
	s.Panel.Base = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(border).
		PaddingLeft(1)
	s.Panel.Title = lipgloss.NewStyle().Foreground(primary).Bold(true)
	s.Panel.Separator = subtle
	s.Panel.Progress = halfMuted
	s.Panel.TaskText = base
	s.Panel.TaskDone = meta.Strikethrough(true)
	s.Panel.IconPending = meta
	s.Panel.IconInProgress = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	s.Panel.IconCompleted = lipgloss.NewStyle().Foreground(green).Bold(true)
	s.Panel.IconBlocked = lipgloss.NewStyle().Foreground(red).Bold(true)

	// Spinner.
	s.SpinnerStyle = lipgloss.NewStyle().Foreground(primary)

	// Markdown.
	s.Markdown = markdownStyle()

	return s
}

func tagStyle(bg, fg color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(bg).Foreground(fg).Padding(0, 1).Bold(true)
}

// markdownStyle creates a glamour-compatible style config.
func markdownStyle() ansi.StyleConfig {
	primary := charmtone.Charple.Hex()
	secondary := charmtone.Dolly.Hex()
	accent := charmtone.Malibu.Hex()
	success := charmtone.Julep.Hex()
	warning := charmtone.Mustard.Hex()
	errorColor := charmtone.Coral.Hex()
	strong := charmtone.Butter.Hex()
	fgBase := charmtone.Ash.Hex()
	fgMuted := charmtone.Squid.Hex()
	fgSoft := charmtone.Smoke.Hex()
	bgSurface := charmtone.Charcoal.Hex()
	border := charmtone.Oyster.Hex()

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:       &fgBase,
				BlockSuffix: "\n",
			},
			Margin: uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &strong,
				BlockSuffix: "\n",
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &primary,
				Prefix:      "# ",
				BlockSuffix: "\n",
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &strong,
				Prefix:      "## ",
				BlockSuffix: "\n",
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &secondary,
				Prefix:      "### ",
				BlockSuffix: "\n",
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &accent,
				Prefix:      "#### ",
				BlockSuffix: "\n",
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &fgBase,
				Prefix:      "##### ",
				BlockSuffix: "\n",
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				Color:       &fgSoft,
				Prefix:      "###### ",
				BlockSuffix: "\n",
			},
		},
		Text: ansi.StylePrimitive{
			Color: &fgBase,
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:           &accent,
				BackgroundColor: &bgSurface,
				Prefix:          " ",
				Suffix:          " ",
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockSuffix: "\n",
				},
				Margin: uintPtr(1),
			},
			Chroma: &ansi.Chroma{
				Background: ansi.StylePrimitive{BackgroundColor: &bgSurface},
				Text:       ansi.StylePrimitive{Color: &fgBase},
				Comment: ansi.StylePrimitive{
					Color:  &fgMuted,
					Italic: boolPtr(true),
				},
				CommentPreproc:      ansi.StylePrimitive{Color: &secondary},
				Keyword:             ansi.StylePrimitive{Color: &primary, Bold: boolPtr(true)},
				KeywordReserved:     ansi.StylePrimitive{Color: &primary, Bold: boolPtr(true)},
				KeywordNamespace:    ansi.StylePrimitive{Color: &secondary},
				KeywordType:         ansi.StylePrimitive{Color: &secondary, Bold: boolPtr(true)},
				Operator:            ansi.StylePrimitive{Color: &fgSoft},
				Punctuation:         ansi.StylePrimitive{Color: &fgMuted},
				Name:                ansi.StylePrimitive{Color: &fgBase},
				NameBuiltin:         ansi.StylePrimitive{Color: &accent},
				NameTag:             ansi.StylePrimitive{Color: &primary, Bold: boolPtr(true)},
				NameAttribute:       ansi.StylePrimitive{Color: &secondary},
				NameClass:           ansi.StylePrimitive{Color: &accent, Bold: boolPtr(true)},
				NameConstant:        ansi.StylePrimitive{Color: &warning},
				NameDecorator:       ansi.StylePrimitive{Color: &primary},
				NameException:       ansi.StylePrimitive{Color: &errorColor},
				NameFunction:        ansi.StylePrimitive{Color: &accent, Bold: boolPtr(true)},
				NameOther:           ansi.StylePrimitive{Color: &fgBase},
				Literal:             ansi.StylePrimitive{Color: &success},
				LiteralNumber:       ansi.StylePrimitive{Color: &warning},
				LiteralDate:         ansi.StylePrimitive{Color: &secondary},
				LiteralString:       ansi.StylePrimitive{Color: &success},
				LiteralStringEscape: ansi.StylePrimitive{Color: &warning},
				GenericDeleted:      ansi.StylePrimitive{Color: &errorColor},
				GenericEmph:         ansi.StylePrimitive{Italic: boolPtr(true)},
				GenericInserted:     ansi.StylePrimitive{Color: &success},
				GenericStrong:       ansi.StylePrimitive{Bold: boolPtr(true)},
				GenericSubheading:   ansi.StylePrimitive{Color: &primary, Bold: boolPtr(true)},
			},
		},
		Link: ansi.StylePrimitive{
			Color:     &accent,
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: &accent,
			Bold:  boolPtr(true),
		},
		Image: ansi.StylePrimitive{
			Color: &secondary,
		},
		ImageText: ansi.StylePrimitive{
			Color: &accent,
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  &secondary,
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold:  boolPtr(true),
			Color: &strong,
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  &border,
			Format: "\n────────\n",
		},
		List: ansi.StyleList{
			StyleBlock:  ansi.StyleBlock{},
			LevelIndent: 2,
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{Color: &fgBase},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: &fgBase},
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
			Faint:      boolPtr(true),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  &fgSoft,
				Italic: boolPtr(true),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:       &fgBase,
					BlockSuffix: "\n",
				},
			},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionList: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n"},
		},
		DefinitionTerm: ansi.StylePrimitive{
			Color: &secondary,
			Bold:  boolPtr(true),
		},
		DefinitionDescription: ansi.StylePrimitive{
			Color:       &fgSoft,
			BlockPrefix: "  ",
		},
		HTMLBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: &fgSoft},
		},
		HTMLSpan: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: &fgSoft},
		},
	}
}

func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }
func boolPtr(b bool) *bool       { return &b }
