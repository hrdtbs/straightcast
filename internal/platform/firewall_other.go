//go:build !windows

package platform

import "context"

// AllowInbound は Windows 以外では何もしません。
func AllowInbound(context.Context, string, int) error { return nil }
