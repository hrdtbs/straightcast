//go:build !windows

package platform

// AllowInbound は Windows 以外では何もしません。
func AllowInbound(string) {}
