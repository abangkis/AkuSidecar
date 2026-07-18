//go:build !windows && !darwin && !linux

package codexruntime

func platformCandidates() []Candidate { return nil }
