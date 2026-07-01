package theme

type Palette struct {
	Background string
	Foreground string
	Accent     string
	Muted      string
	Warning    string
	Error      string
	Success    string
}

func DefaultPalette() Palette {
	return Palette{
		Background: "#111018",
		Foreground: "#f6f2ff",
		Accent:     "#9146ff",
		Muted:      "#9d97aa",
		Warning:    "#f5c542",
		Error:      "#ff5c7a",
		Success:    "#4ade80",
	}
}
