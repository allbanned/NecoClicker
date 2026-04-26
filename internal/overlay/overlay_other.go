//go:build !windows

package overlay

type Overlay struct{}

func New() *Overlay                  { return &Overlay{} }
func (o *Overlay) Start() error      { return nil }
func (o *Overlay) Stop()             {}
func (o *Overlay) Ping(x, y int)     {}
func (o *Overlay) Enable(v bool)     {}
func (o *Overlay) IsEnabled() bool   { return false }
func (o *Overlay) SetColor(r, g, b byte) {}
