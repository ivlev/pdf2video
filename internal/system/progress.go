package system

import (
	"fmt"
	"strings"
	"time"
)

type ProgressBar struct {
	Total      int
	Current    int
	Width      int
	StartTime  time.Time
	LastUpdate time.Time
	Label      string
}

func NewProgressBar(total int, label string) *ProgressBar {
	return &ProgressBar{
		Total:     total,
		Width:     40,
		StartTime: time.Now(),
		Label:     label,
	}
}

func (p *ProgressBar) Update(current int) {
	if current > p.Total {
		current = p.Total
	}
	p.Current = current
	p.render()
}

func (p *ProgressBar) Increment() {
	p.Update(p.Current + 1)
}

func (p *ProgressBar) render() {
	percent := float64(p.Current) / float64(p.Total)
	filledWidth := int(percent * float64(p.Width))
	if filledWidth > p.Width {
		filledWidth = p.Width
	}

	bar := strings.Repeat("█", filledWidth) + strings.Repeat("░", p.Width-filledWidth)

	elapsed := time.Since(p.StartTime).Seconds()
	var eta float64
	if p.Current > 0 {
		eta = (elapsed / float64(p.Current)) * float64(p.Total-p.Current)
	}

	fmt.Printf("\r%s [%s] %3.0f%% | ETA: %s   ",
		p.Label, bar, percent*100, formatDuration(eta))

	if p.Current == p.Total {
		fmt.Println()
	}
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
