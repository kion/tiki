//go:build !linux

package livereload

func explainWatchErr(err error) error { return err }
