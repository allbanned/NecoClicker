//go:build !windows

package winmouse

func GetCursor() (int, int)                   { return 0, 0 }
func SetCursor(x, y int)                      {}
func Click(button string, x, y int, cur bool) {}
func MouseDown(button string)                 {}
func MouseUp(button string)                   {}
