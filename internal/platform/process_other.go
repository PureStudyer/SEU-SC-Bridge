//go:build !windows

package platform

func processStartTime() (uint64, error) { return 0, nil }

func RecoverStaleAgent(Paths, string) (bool, error) { return false, nil }
