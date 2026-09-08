package fiber

import (
	"fmt"
	"sort"
	"strings"
)

// uiComponent is one component's class lists. Base/Variants/Sizes are
// cva axes and compose; Parts replace the base.
//
// There is no tailwind-merge here, so a base must never set a property
// its variants or sizes also set: two competing utilities resolve by
// Tailwind's output order, not the order written. Parts must stay
// space-free where classList.add() consumes them (combobox
// "item-active"), which throws on a value containing a space.
type uiComponent struct {
	Base     string
	Variants map[string]string
	Sizes    map[string]string
	Parts    map[string]string
}

// uiRegistry resolves shadcn's cva lookups at template-execution time,
// via the "ui" template func: {{ui "button" "outline" "size-sm"}}.
//
// Colors are the CSS variables from admin/theme.html rather than a
// literal palette, which is what makes the admin themeable. Sizes are
// prefixed "size-" because shadcn has both a variant and a size named
// "default"; the prefix also tells the resolver which axis was given.
var uiRegistry = map[string]uiComponent{

	"button": {
		// No height, padding, or background: those live in Sizes and
		// Variants, and a base must not fight its own axes.
		Base: "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-sm font-medium tracking-wide transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50",
		Variants: map[string]string{
			// shadcn's six stock button variants, verbatim in intent.
			"default":     "bg-primary text-primary-foreground hover:bg-primary/90",
			"destructive": "bg-destructive text-destructive-foreground hover:bg-destructive/90",
			"outline":     "border border-input bg-background text-foreground hover:bg-accent hover:text-accent-foreground",
			"secondary":   "bg-secondary text-secondary-foreground hover:bg-secondary/80",
			"ghost":       "text-foreground hover:bg-accent hover:text-accent-foreground",
			"link":        "text-primary underline-offset-4 hover:underline",
			// shadcn ships no destructive counterpart to `outline`. A
			// detail page's Delete must match the weight of the outline
			// Edit beside it (solid `destructive` is reserved for the
			// confirmation page's submit), and a row's icon-only Delete
			// wants the colour with no chrome at all.
			"destructive-outline": "border border-destructive/40 bg-background text-destructive hover:bg-destructive hover:text-destructive-foreground",
			"ghost-destructive":   "text-destructive hover:bg-destructive/10 hover:text-destructive",
			// Low-emphasis muted ghost: the view/edit row buttons, the
			// sidebar close, the theme toggle.
			"ghost-muted": "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
		},
		Sizes: map[string]string{
			"size-default": "h-10 px-4 py-2",
			"size-sm":      "h-9 px-3",
			"size-xs":      "h-8 px-2.5 text-xs",
			"size-lg":      "h-11 px-8",
			"size-icon":    "h-9 w-9 shrink-0",
			"size-icon-sm": "h-8 w-8 shrink-0",
			"size-icon-xs": "h-7 w-7 shrink-0",
		},
	},

	"input": {
		Base: "flex w-full rounded-md border border-input bg-background text-sm text-foreground ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
		Sizes: map[string]string{
			"size-default": "h-10 px-3 py-2",
			"size-sm":      "h-9 px-2",
		},
		Parts: map[string]string{
			// A standalone (not composed) control for an input sitting
			// inside an already-bordered wrapper -- the combobox's search
			// field -- so it contributes no border, ring, or background of
			// its own.
			"bare": "flex h-10 w-full bg-transparent p-0 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50",
		},
	},

	"textarea": {
		Base:  "flex w-full rounded-md border border-input bg-background text-sm text-foreground ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
		Sizes: map[string]string{"size-default": "min-h-[80px] px-3 py-2"},
	},

	// A native <select>, not shadcn's Radix-backed SelectTrigger: a
	// custom listbox does not post its value with a plain form submit,
	// and the admin's forms have to keep working without JS. Styled to
	// match the shadcn trigger (same height, border, ring).
	"select": {
		Base: "flex w-full items-center rounded-md border border-input bg-background text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
		Sizes: map[string]string{
			"size-default": "h-10 px-3 py-2",
			"size-sm":      "h-9 px-2",
			// <select multiple> sizes itself by its `size` attribute.
			"size-auto": "h-auto px-3 py-2",
		},
		Parts: map[string]string{
			// Table-cell flavours. The base's w-full leaves a <td> select
			// with no intrinsic width, so the column ignores the options
			// and clips the widest ("Enterprise" as "Enterpri"). w-auto
			// lets the browser measure them; max-w-full caps the damage
			// one long option can do. Spelled out in full because a part
			// cannot compose with a size.
			"cell":       "flex h-9 w-auto max-w-full items-center rounded-md border border-input bg-background px-2 text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
			"cell-multi": "flex h-auto w-auto max-w-full items-center rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
		},
	},

	"label": {
		Base: "block text-sm font-medium leading-none text-foreground peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
	},

	// appearance-none is what makes the rest apply: a native checkbox
	// draws itself and ignores border/radius/background. The check glyph
	// is a background image on .ui-checkbox in theme.html.
	"checkbox": {
		Base: "ui-checkbox peer size-4 shrink-0 appearance-none rounded-[4px] border border-input bg-background shadow-sm outline-none transition-shadow checked:border-primary checked:bg-primary focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50",
	},

	"radio": {
		Base: "h-4 w-4 shrink-0 border-input bg-background accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
	},

	// Built on a native checkbox rather than a button, so it toggles
	// with no JavaScript and posts like the checkbox it replaces. track
	// and thumb are siblings of the input, not children: `peer` compiles
	// to a sibling combinator and would not cross into a child.
	"switch": {
		Base: "relative inline-flex shrink-0 cursor-pointer items-center",
		Parts: map[string]string{
			"input": "peer sr-only",
			"track": "h-5 w-9 rounded-full border-2 border-transparent bg-input transition-colors peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-ring peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background peer-disabled:cursor-not-allowed peer-disabled:opacity-50",
			"thumb": "pointer-events-none absolute left-0.5 block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform peer-checked:translate-x-4",
		},
	},

	"badge": {
		Base: "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none",
		Variants: map[string]string{
			"default":     "border-transparent bg-primary text-primary-foreground",
			"secondary":   "border-transparent bg-secondary text-secondary-foreground",
			"destructive": "border-transparent bg-destructive text-destructive-foreground",
			"outline":     "border-border text-foreground",
		},
	},

	"card": {
		Base: "rounded-lg border border-border bg-card text-card-foreground shadow-sm",
		Parts: map[string]string{
			"header":      "flex flex-col gap-1.5 p-6",
			"title":       "text-sm font-medium leading-none tracking-tight text-muted-foreground",
			"description": "text-sm text-muted-foreground",
			"content":     "p-6 pt-0",
			"footer":      "flex items-center p-6 pt-0",
		},
	},

	// shadcn's login-04 block. Its social buttons, "Sign up" and "Forgot
	// your password?" are deliberately absent: the framework has no
	// route behind any of them. login-04 fills the right column with a
	// photograph, which a framework cannot ship, so "aside" styles a
	// typographic panel instead.
	"login": {
		Parts: map[string]string{
			"page":      "flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10",
			"container": "flex w-full max-w-sm flex-col gap-6 md:max-w-3xl",
			// Overrides card's own padding: in this block the columns
			// pad themselves, so the card must not.
			"grid":     "grid p-0 md:grid-cols-2",
			"form":     "p-6 md:p-8",
			"stack":    "flex flex-col gap-6",
			"heading":  "flex flex-col items-center text-center",
			"title":    "text-2xl font-bold",
			"subtitle": "text-balance text-muted-foreground",
			"group":    "grid gap-2",
			// bg-muted is the page's own colour, so without login-04's
			// photograph over it the card read as half-width with text
			// floating beside it. The chart tokens are defined in both
			// themes, so a tint from them stays distinct without
			// hardcoding a colour.
			"aside": "relative hidden border-l border-border bg-gradient-to-br from-chart-1/20 via-chart-6/10 to-chart-3/20 md:block",
			"panel": "absolute inset-0 flex flex-col items-center justify-center gap-3 p-8 text-center",
			"logo":  "h-12 w-auto opacity-80",
			"brand": "text-xl font-semibold text-foreground",
		},
	},

	// The 401/403/404 page.
	"error": {
		Parts: map[string]string{
			"page":    "flex min-h-svh flex-col items-center justify-center bg-muted p-6 md:p-10",
			"card":    "flex w-full max-w-md flex-col items-center gap-3 rounded-lg border border-border bg-card p-8 text-center text-card-foreground shadow-sm",
			"status":  "text-5xl font-bold tracking-tight text-muted-foreground/40",
			"title":   "text-xl font-semibold",
			"message": "text-balance text-sm text-muted-foreground",
		},
	},

	// The native overflow box, with the scrollbar restyled by the
	// .ui-scroll-area rules in theme.html (a scrollbar cannot be
	// expressed as Tailwind utilities without a plugin). The axis is the
	// whole decision, so there are two parts and no base.
	"scroll-area": {
		Parts: map[string]string{
			"x": "ui-scroll-area block max-w-xs overflow-x-auto overflow-y-hidden whitespace-nowrap",
			"y": "ui-scroll-area block max-h-40 overflow-y-auto overflow-x-hidden",
		},
	},

	"alert": {
		// `border` with no color; the variants supply the color.
		Base: "relative w-full rounded-lg border p-4",
		Variants: map[string]string{
			"default":     "border-border bg-card text-card-foreground",
			"destructive": "border-destructive/50 bg-destructive/10 text-destructive dark:border-destructive",
		},
		Parts: map[string]string{
			"title":       "mb-1 text-sm font-medium leading-none tracking-tight",
			"description": "text-sm opacity-90",
		},
	},

	"separator": {
		Base: "shrink-0 bg-border",
		Variants: map[string]string{
			"horizontal": "h-px w-full",
			"vertical":   "h-full w-px",
		},
	},

	"skeleton": {Base: "animate-pulse rounded-md bg-muted"},

	"avatar": {
		Base: "relative flex h-9 w-9 shrink-0 overflow-hidden rounded-full bg-muted",
		Parts: map[string]string{
			"image":    "aspect-square h-full w-full object-cover",
			"fallback": "flex h-full w-full items-center justify-center bg-muted text-xs font-medium text-muted-foreground",
		},
	},

	// Shared text roles, so "the muted small text" is one decision
	// rather than a repeated literal in twelve templates.
	"text": {
		Parts: map[string]string{
			"muted":       "text-sm text-muted-foreground",
			"muted-xs":    "text-xs text-muted-foreground",
			"empty":       "text-sm text-muted-foreground",
			"placeholder": "text-muted-foreground",
			"heading":     "text-sm font-semibold text-foreground",
			"label-caps":  "text-xs font-semibold uppercase tracking-wide text-muted-foreground",
			"metric":      "text-3xl font-semibold text-foreground",
			"link":        "text-primary underline-offset-4 hover:underline",
			"error":       "text-xs font-medium text-destructive",
		},
	},

	"dialog": {
		Parts: map[string]string{
			"overlay":     "fixed inset-0 bg-black/80",
			"container":   "fixed inset-0 flex items-center justify-center p-4",
			"content":     "relative w-full max-w-lg rounded-lg border border-border bg-background p-6 text-foreground shadow-lg",
			"content-sm":  "relative w-full max-w-sm rounded-lg border border-border bg-background p-6 text-foreground shadow-lg",
			"title":       "text-base font-semibold leading-none tracking-tight text-foreground",
			"description": "mt-2 text-sm text-muted-foreground",
			"footer":      "mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
		},
	},

	"dropdown": {
		Parts: map[string]string{
			"content":   "z-50 min-w-[10rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md",
			"item":      "relative flex w-full cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none",
			"label":     "px-2 py-1.5 text-xs font-semibold text-muted-foreground",
			"separator": "-mx-1 my-1 h-px bg-muted",
		},
	},

	"popover": {
		Parts: map[string]string{
			"content": "z-50 w-72 rounded-md border border-border bg-popover p-4 text-popover-foreground shadow-md outline-none",
		},
	},

	"tooltip": {
		Parts: map[string]string{
			"content": "z-50 overflow-hidden rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-primary-foreground shadow-md",
		},
	},

	// A restyle of the existing PinesUI-derived queue in toasts.html,
	// which already has Sonner's shape (teleported stack, per-type icon,
	// auto-dismiss).
	"toast": {
		Parts: map[string]string{
			// pointer-events-none is load-bearing: the viewport spans a
			// strip of the screen even when empty, and record pages put
			// a sticky action bar in that same corner, so it would
			// otherwise swallow clicks on Save. Each toast re-enables
			// pointer events for itself.
			"list": "pointer-events-none fixed inset-x-0 bottom-0 z-[100] flex max-h-screen " +
				"flex-col gap-2 p-4 sm:inset-x-auto sm:right-0 sm:bottom-0 md:max-w-[420px]",
			"root": "pointer-events-auto group relative flex w-full items-start gap-3 overflow-hidden " +
				"rounded-md border border-border bg-background p-4 pr-8 text-foreground shadow-lg",
			"title":       "text-sm font-semibold leading-none",
			"description": "mt-1.5 text-sm leading-snug opacity-90",
			"close": "absolute right-2 top-2 rounded-md p-1 text-foreground/50 opacity-0 " +
				"transition-opacity hover:text-foreground focus:opacity-100 focus:outline-none " +
				"focus:ring-2 focus:ring-ring group-hover:opacity-100",
		},
	},

	"sheet": {
		Parts: map[string]string{
			"overlay": "fixed inset-0 z-40 bg-black/60",
			// Deliberately carries no background or width: the sidebar
			// composes this with `ui "sidebar"`, and two competing
			// bg-*/w-* utilities resolve by Tailwind's output order rather
			// than intent. Panels that aren't the sidebar add "panel".
			"content":     "fixed z-50 shadow-lg transition-transform duration-300 ease-in-out",
			"panel":       "bg-background",
			"side-left":   "inset-y-0 left-0 h-full border-r border-border",
			"side-right":  "inset-y-0 right-0 h-full border-l border-border",
			"width-panel": "w-72",
		},
	},

	// shadcn's sidebar-07 block, at its exact widths (16rem open, 3rem
	// collapsed, 18rem mobile sheet). Its --sidebar-* colour scale is
	// deliberately not reproduced: the Zinc values are within a hair of
	// card/accent/border, and restyling the admin must mean editing
	// theme.html's variables and nothing else.
	"sidebar": {
		Base: "flex h-full shrink-0 flex-col border-r border-border bg-card " +
			"transition-[width] duration-200 ease-linear",
		Parts: map[string]string{
			"expanded": "w-64",
			// 3rem exactly: p-2 either side of a size-8 icon button, so
			// the icon column doesn't shift during the transition.
			"collapsed":   "w-12",
			"header":      "flex flex-col gap-2 p-2",
			"content":     "flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto overflow-x-hidden p-2",
			"footer":      "flex flex-col gap-2 border-t border-border p-2",
			"group":       "flex w-full min-w-0 flex-col",
			"group-label": "flex h-8 shrink-0 items-center rounded-md px-2 text-xs font-medium text-muted-foreground/70",
			"menu":        "flex w-full min-w-0 flex-col gap-1",
			// The sub-menu's left rule is what makes nesting legible once
			// a group is open; it has no icon column of its own.
			"menu-sub": "mx-3.5 flex min-w-0 flex-col gap-1 border-l border-border px-2.5 py-0.5",
			// SidebarRail: the hairline strip on the sidebar's edge that
			// toggles it. Invisible until hovered, hence ::after not a border.
			"rail": "absolute inset-y-0 -right-2 z-20 hidden w-4 cursor-w-resize md:block " +
				"after:absolute after:inset-y-0 after:left-1/2 after:w-px " +
				"after:transition-colors hover:after:bg-border",
			// SidebarInset: the content column beside the sidebar.
			"inset": "relative flex min-h-0 min-w-0 flex-1 flex-col bg-background",
			// The block's h-16 header: trigger, separator, breadcrumbs.
			"topbar": "flex h-16 shrink-0 items-center gap-2 border-b border-border px-4",
		},
	},

	// SidebarMenuButton. p-2 + a size-4 icon is exactly the collapsed
	// sidebar's 3rem, so nothing shifts horizontally as it animates --
	// only the label clips away.
	"nav-item": {
		// No background here: active/inactive supply it.
		Base: "flex w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
		Variants: map[string]string{
			"active":   "bg-accent font-medium text-accent-foreground",
			"inactive": "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground",
		},
		Sizes: map[string]string{
			// Base carries padding, not height, so these don't fight it.
			"size-default": "h-8",
			// The taller brand and user buttons at the sidebar's two ends.
			"size-lg": "h-12",
		},
	},

	"tabs": {
		Parts: map[string]string{
			"list":        "inline-flex h-9 items-center justify-center gap-1 rounded-lg bg-muted p-1 text-muted-foreground",
			"trigger":     "inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",
			"trigger-on":  "bg-background text-foreground shadow-sm",
			"trigger-off": "text-muted-foreground hover:text-foreground",
			// Underline flavor, for the dashboard's Tabs container widget
			// -- a pill row would fight the card it sits inside.
			"underline-list":        "mb-3 flex gap-1 border-b border-border",
			"underline-trigger":     "-mb-px border-b-2 px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
			"underline-trigger-on":  "border-primary text-foreground",
			"underline-trigger-off": "border-transparent text-muted-foreground hover:text-foreground",
		},
	},

	"accordion": {
		Parts: map[string]string{
			"item":    "border-b border-border",
			"trigger": "flex w-full flex-1 items-center justify-between gap-3 py-3 text-sm font-medium text-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
			"content": "overflow-hidden pb-3 text-sm text-muted-foreground",
			"chevron": "shrink-0 transition-transform duration-200",
		},
	},

	"breadcrumb": {
		Base: "flex flex-wrap items-center gap-1.5 text-sm text-muted-foreground",
		Parts: map[string]string{
			"item":      "flex items-center gap-1.5",
			"link":      "rounded transition-colors hover:text-foreground",
			"page":      "cursor-default font-semibold text-foreground",
			"plain":     "cursor-default",
			"separator": "text-border",
		},
	},

	// Toolbar + footer of the list table, from shadcn's Tasks example:
	// search and filters left, page actions right; below, the
	// DataTablePagination row.
	"toolbar": {
		// Stacked until lg, a row from lg up. lg, not sm, because the
		// sidebar becomes a static 16rem column at md: the content area
		// *shrinks* there (735px -> 549px), and measured, the five
		// controls only stop wrapping into a ragged block at ~1024px.
		Base: "flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between",
		Parts: map[string]string{
			// Every control carries its own "item" to fill its line while
			// stacked: stretch alone cannot, since several are buttons
			// wrapped in a <form> or a positioning <div>. flex-1 so the
			// filter cluster takes the slack once they are rows.
			"filters": "flex flex-1 flex-col gap-2 lg:flex-row lg:flex-wrap lg:items-center",
			"actions": "flex flex-col gap-2 lg:flex-row lg:items-center",
			"item":    "w-full lg:w-auto",
			// Centred content in a full-width bar reads as floating, so
			// while stacked the label goes hard left and the icon hard
			// right. "item-label" takes the slack (which also leaves the
			// base's justify-center nothing to centre, so the two never
			// fight); "item-icon" moves a leading icon to the trailing
			// edge without reordering the markup. Both revert at lg.
			"item-label": "flex-1 text-left lg:flex-none",
			"item-icon":  "order-last lg:order-none",
		},
	},

	// The filter drawer: one Filters trigger in the toolbar opening a
	// sheet from the right. One trigger stays one trigger however many
	// filters a ModelAdmin declares, which per-filter dropdowns don't.
	"filter-panel": {
		Parts: map[string]string{
			"header": "flex items-center justify-between gap-2 border-b border-border px-4 py-3",
			"title":  "text-sm font-semibold text-foreground",
			"body":   "flex-1 space-y-5 overflow-y-auto px-4 py-4",
			"group-label": "mb-1.5 text-xs font-semibold uppercase tracking-wide " +
				"text-muted-foreground",
			"choice": "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors " +
				"hover:bg-accent hover:text-accent-foreground",
			"choice-active":   "bg-accent font-medium text-accent-foreground",
			"choice-inactive": "text-muted-foreground",
			"footer":          "border-t border-border px-4 py-3",
			// The count badge on the trigger, shown only while filters
			// are actually narrowing the list.
			"count": "rounded-sm px-1 font-mono text-xs font-normal",
		},
	},

	"pagination": {
		Base: "flex flex-col items-center gap-3 sm:flex-row sm:justify-between",
		Parts: map[string]string{
			"list":     "flex h-9 items-center divide-x divide-border overflow-hidden rounded-md border border-border bg-card text-sm leading-tight text-muted-foreground",
			"link":     "relative inline-flex h-full items-center px-3 transition-colors hover:bg-accent hover:text-accent-foreground",
			"disabled": "pointer-events-none text-muted-foreground/40",
			"current":  "hidden h-full items-center px-3 font-medium text-foreground sm:flex",
			// The selection count, which stays visible (as "0 of N")
			// even with nothing selected, exactly as the example does.
			"selection":      "flex-1 text-sm text-muted-foreground",
			"controls":       "flex items-center gap-4 lg:gap-8",
			"rows-per-page":  "flex items-center gap-2 text-sm font-medium",
			"page-indicator": "flex w-[100px] items-center justify-center text-sm font-medium",
			"jumps":          "flex items-center gap-2",
			// The two outer jumps (first/last) are hidden on small
			// screens in the example -- prev/next are enough there.
			"jump-edge": "hidden h-8 w-8 p-0 lg:flex",
			"jump":      "h-8 w-8 p-0",
		},
	},

	"table": {
		Base: "w-full min-w-full caption-bottom text-sm",
		Parts: map[string]string{
			"wrapper": "overflow-hidden rounded-lg border border-border bg-card",
			"scroll":  "overflow-x-auto",
			// The "select all N matching" strip: only ever visible once
			// a whole page is ticked, so it reads as a follow-up
			// question rather than permanent chrome.
			"select-all":        "flex flex-wrap items-center justify-center gap-2 border-b border-border bg-muted/40 px-4 py-2 text-center text-sm text-muted-foreground",
			"select-all-action": "font-medium text-foreground underline underline-offset-2 hover:no-underline",
			"head":              "[&_tr]:border-b [&_tr]:border-border",
			"th":                "px-4 py-2.5 text-left align-middle font-medium whitespace-nowrap text-muted-foreground",
			"body":              "divide-y divide-border",
			// data-[state=selected], from shadcn/ui's own TableRow: a
			// checked row-checkbox (list.html) tints the whole row, the
			// same as a hover, so a selection reads at a glance instead
			// of only through the toolbar's "N of M selected" count.
			"row":   "text-foreground transition-colors hover:bg-muted/50 has-[.row-checkbox:checked]:bg-muted",
			"cell":  "px-4 py-2.5 align-middle text-sm whitespace-nowrap",
			"empty": "px-4 py-6 text-sm text-muted-foreground",
			// Compact flavor for the dashboard Table widget and tabular
			// inlines, which sit inside an existing card.
			"th-compact":   "px-3 py-2 text-left text-xs font-medium whitespace-nowrap text-muted-foreground",
			"cell-compact": "px-3 py-2 align-middle whitespace-nowrap",
		},
	},

	// The label + control + description + error unit. shadcn calls this
	// FormItem/FormLabel/FormDescription/FormMessage; here it is what
	// wraps every generated form input.
	"field": {
		Base: "mb-4",
		Parts: map[string]string{
			"label":       "block text-sm font-medium leading-none text-foreground",
			"required":    "ml-0.5 text-destructive",
			"control":     "mt-1.5",
			"description": "mt-1.5 text-xs text-muted-foreground",
			"message":     "mt-1.5 text-xs font-medium text-destructive",
			// A boolean puts its control on the same row as its name.
			// items-start, not items-center: with a description the text
			// block is two lines and centring leaves the toggle sitting
			// low. row-label's 20px line box matches the switch height,
			// so top-aligning reads as centred either way.
			"row":       "flex items-start gap-2",
			"row-text":  "min-w-0",
			"row-label": "leading-5",
			// A read-only field's value, shown instead of a control.
			// Deliberately not input-shaped: a disabled-looking box
			// invites clicking at it, a plain value does not.
			"readonly": "py-1.5 text-sm text-foreground",
		},
	},

	"combobox": {
		Base: "relative",
		Parts: map[string]string{
			"trigger": "flex items-center gap-2 rounded-md border border-input bg-background px-3 ring-offset-background focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2",
			"content": "absolute z-50 mt-1 max-h-[280px] w-full overflow-y-auto rounded-md border border-border bg-popover py-1 text-sm text-popover-foreground shadow-md",
			"item":    "relative flex cursor-pointer select-none items-center rounded-sm px-3 py-1.5 text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
			// Handed to classList.add()/remove() by the arrow-key
			// handler, so this must stay a single space-free class.
			"item-active": "bg-accent",
			"empty":       "px-3 py-1.5 text-muted-foreground",
			"icon":        "w-4 h-4 shrink-0 text-muted-foreground",
		},
	},

	// The many-to-many control: shadcn's Combobox/Command idiom over the
	// relation's options, selection shown as removable chips. Django's
	// filter_horizontal without the two-pane layout, which needs width
	// this form column doesn't have.
	"multi-select": {
		Parts: map[string]string{
			// min-h matches the single Select's h-10 so a field with
			// nothing chosen lines up with its neighbours; it grows from
			// there as chips wrap onto more lines.
			"trigger": "min-h-10 justify-between gap-2 text-left",
			"values":  "flex flex-1 flex-wrap items-center gap-1",
			"chip":    "gap-1 py-0.5 pr-1 pl-2 font-normal",
			// The chip's own remove affordance. Not a <button>: the chip
			// lives inside the trigger <button>, and HTML forbids nesting
			// one button in another.
			"chip-remove": "rounded-sm p-0.5 transition-colors hover:bg-background/60",
			"search":      "flex items-center gap-2 border-b border-border px-3",
			"list":        "max-h-56 overflow-y-auto py-1",
		},
	},

	"calendar": {
		Base: "w-auto p-3",
		Parts: map[string]string{
			"header":  "flex items-center justify-between gap-2 pb-2",
			"caption": "text-sm font-medium text-foreground",
			"nav":     "inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
			"grid":    "w-full border-collapse",
			"weekday": "h-8 w-8 p-0 text-center text-[0.7rem] font-normal text-muted-foreground",
			// "day" carries no background so the three state parts below
			// can be layered on via an Alpine :class binding.
			"day":          "h-8 w-8 rounded-md p-0 text-center text-sm font-normal text-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
			"day-selected": "bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground",
			"day-today":    "border border-border font-semibold",
			"day-outside":  "text-muted-foreground/50",
		},
	},

	"slider": {
		Base: "relative flex w-full touch-none select-none items-center",
		Parts: map[string]string{
			// A native range input, tinted with accent-color -- same
			// no-JS-form reasoning as checkbox/radio.
			"track":  "h-2 w-full cursor-pointer appearance-none rounded-full bg-secondary accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
			"output": "w-12 shrink-0 text-right text-sm tabular-nums text-muted-foreground",
		},
	},

	"widget": {
		Base: "rounded-lg border border-border bg-card p-4 text-card-foreground shadow-sm sm:p-6",
		Parts: map[string]string{
			"header":    "mb-3 flex items-center gap-3",
			"icon":      "inline-flex shrink-0 items-center justify-center rounded-lg bg-muted p-2 text-foreground",
			"title":     "text-sm font-medium text-muted-foreground",
			"span-lg":   "sm:col-span-2 xl:col-span-3",
			"bar-track": "h-2 w-full rounded-full bg-muted",
			"bar-fill":  "h-2 rounded-full bg-primary",
		},
	},

	// Form fieldset (Django's `fieldsets`): a titled, collapsible group
	// of fields inside the record form. A group with no title renders
	// bare -- no header, no border -- so the default single-group case
	// looks exactly like the flat form it replaces.
	"fieldset": {
		Base: "mb-6 last:mb-0 rounded-lg border border-border bg-muted/20",
		Parts: map[string]string{
			"trigger":     "flex w-full items-center justify-between gap-2 rounded-t-lg px-4 py-3 text-left transition-colors hover:bg-muted/40",
			"title":       "text-sm font-semibold text-foreground",
			"description": "px-4 pb-2 text-sm text-muted-foreground",
			"body":        "border-t border-border p-4",
			"icon":        "size-4 shrink-0 text-muted-foreground transition-transform",
		},
	},
	// The detail page's History panel: a compact recent-activity list,
	// shown only when the configured audit logger can read back.
	"history": {
		Base: "mt-6 rounded-lg border border-border bg-card",
		Parts: map[string]string{
			"title": "border-b border-border px-4 py-2.5 text-sm font-semibold text-foreground",
			"row":   "flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 px-4 py-2 text-sm",
			"when":  "text-muted-foreground tabular-nums",
			"who":   "font-medium text-foreground",
			"what":  "text-muted-foreground",
			"empty": "px-4 py-3 text-sm text-muted-foreground",
		},
	},
	"panel": {
		Base: "rounded-lg border border-border bg-card p-4 text-card-foreground",
		Parts: map[string]string{
			"dashed": "rounded-lg border border-dashed border-border bg-muted/40 p-4 text-sm text-muted-foreground",
			"form":   "w-full rounded-lg border border-border bg-card p-7 text-card-foreground shadow-sm",
		},
	},
	// Record-page shell for detail/create/edit: one full-height column
	// holding the record and an action bar pinned to the bottom of the
	// viewport. Used by admin/resource/detail.html and admin/resource/form.html so the
	// two never disagree about width or alignment.
	"page": {
		// min-h-full (not h-full) is what lets the column grow past the
		// viewport for a long record instead of clipping it.
		Base: "flex min-h-full flex-col",
		Parts: map[string]string{
			// my-auto gives "centred when it fits, scrolls when it
			// doesn't" with no media query: free space splits above and
			// below, and collapses to 0 once the content overflows.
			"body": "mx-auto my-auto w-full max-w-xl space-y-4",
			// max-w-xl is the measure for a column of form fields, but a
			// tabular inline is a table: squeezing one into 576px is
			// what clipped its own row actions, so a page holding one
			// gets the wider cap.
			"body-wide": "mx-auto my-auto w-full max-w-5xl space-y-4",
			// sticky rather than fixed: it reserves its own space inside
			// <main>'s scroll container, so it needs no sidebar-width
			// offset and no compensating bottom padding. The negative
			// margins cancel <main>'s p-4 so it spans the full width.
			"actions": "sticky bottom-0 z-10 -mx-4 -mb-4 mt-4 border-t border-border " +
				"bg-background/95 px-4 py-3 backdrop-blur",
			// Matched to "body"'s max-w-xl so the buttons line up with
			// the record. flex-col-reverse below sm floats the primary
			// group to the top, so Delete is never the button under your
			// thumb when the bar stacks.
			"actions-inner": "mx-auto flex w-full max-w-xl flex-col-reverse gap-2 " +
				"sm:flex-row sm:items-center",
			// Matched to "body-wide" for the same reason "actions-inner"
			// is matched to "body": the bar has to line up with the
			// card above it, so widening one without the other leaves
			// the buttons drifting.
			"actions-inner-wide": "mx-auto flex w-full max-w-5xl flex-col-reverse gap-2 " +
				"sm:flex-row sm:items-center",
			// The right-hand group. sm:ml-auto does the separating, so
			// the bar reads Delete-left / everything-else-right when a
			// Delete is present and simply right-aligns when it isn't
			// (create, and detail once Delete moved to the edit page).
			"actions-primary": "flex flex-col gap-2 sm:ml-auto sm:flex-row",
		},
	},
}

// uiClasses resolves a component's class string from uiRegistry, and is
// registered as the "ui" template func.
//
// A modifier naming a Part resolves to that part alone; Variant and Size
// modifiers compose with Base, defaulting whichever axis was left out.
// An unknown name is an error rather than a silent skip: a template func
// returning non-nil aborts ExecuteTemplate, so a typo fails loudly in
// the render tests instead of shipping an unstyled button.
func uiClasses(component string, modifiers ...string) (string, error) {
	spec, ok := uiRegistry[component]
	if !ok {
		return "", fmt.Errorf("polyadmin: unknown ui component %q (known: %s)", component, knownUIComponents())
	}

	// A single Part reference stands alone.
	if len(modifiers) == 1 {
		if part, ok := spec.Parts[modifiers[0]]; ok {
			return part, nil
		}
	}

	var parts []string
	if spec.Base != "" {
		parts = append(parts, spec.Base)
	}

	var hasSize, hasVariant bool
	for _, m := range modifiers {
		if strings.HasPrefix(m, "size-") {
			hasSize = true
		} else {
			hasVariant = true
		}
	}
	if !hasVariant {
		if def, ok := spec.Variants["default"]; ok {
			parts = append(parts, def)
		}
	}
	if !hasSize {
		if def, ok := spec.Sizes["size-default"]; ok {
			parts = append(parts, def)
		}
	}

	for _, m := range modifiers {
		switch value, found := spec.Variants[m]; {
		case found:
			parts = append(parts, value)
		default:
			if value, found := spec.Sizes[m]; found {
				parts = append(parts, value)
				continue
			}
			if _, found := spec.Parts[m]; found {
				return "", fmt.Errorf(
					"polyadmin: ui %q part %q cannot be combined with other modifiers (a part replaces the base; request it on its own)",
					component, m)
			}
			return "", fmt.Errorf("polyadmin: ui component %q has no modifier %q (known: %s)",
				component, m, knownUIModifiers(spec))
		}
	}

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " "), nil
}

func knownUIComponents() string {
	names := make([]string, 0, len(uiRegistry))
	for name := range uiRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func knownUIModifiers(spec uiComponent) string {
	names := make([]string, 0, len(spec.Variants)+len(spec.Sizes)+len(spec.Parts))
	for _, group := range []map[string]string{spec.Variants, spec.Sizes, spec.Parts} {
		for name := range group {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
