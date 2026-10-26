//go:build windows && auth_session_dev

package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// All profiles here are empty synthetic directories under the test runner's
// project-owned temporary root. No native process or real QA manifest is used.
func retainedSyntheticParent(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	lease, err := acquireXRetainedQAProfileAtParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(lease.Root, "shared"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = lease.RememberProfile(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Release(true); err != nil {
		t.Fatal(err)
	}
	return parent
}

func TestXRetainedSharedCallerExclusion(t *testing.T) {
	for _, existingFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-first", true: "application-first"}[existingFirst], func(t *testing.T) {
			parent := retainedSyntheticParent(t)
			first, err := acquireXRetainedProfileAtParent(parent, existingFirst)
			if err != nil {
				t.Fatal(err)
			}
			if other, e := acquireXRetainedProfileAtParent(parent, !existingFirst); e == nil {
				other.Release(false)
				t.Fatal("competing caller acquired legacy manifest lock")
			}
			if first.Validate() != nil || !first.Matches(first.Root, filepath.Join(first.Root, "shared")) {
				t.Fatal("held scope unavailable")
			}
			if first.Matches(first.Root, filepath.Join(first.Root, "other")) {
				t.Fatal("wrong profile accepted")
			}
			if err = first.Close(); err != nil {
				t.Fatal(err)
			}
			second, err := acquireXRetainedProfileAtParent(parent, !existingFirst)
			if err != nil {
				t.Fatal(err)
			}
			if second.descriptor() != first.descriptor() {
				t.Fatal("in-place identities changed")
			}
			if err = second.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestXRetainedExistingOnlyAndDirtyFence(t *testing.T) {
	parent := t.TempDir()
	if _, err := acquireXRetainedProfileAtParent(parent, true); err == nil {
		t.Fatal("missing manifest accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, xRetainedQAProfileManifestName)); !os.IsNotExist(err) {
		t.Fatal("existing-only acquisition created a manifest")
	}
	lease, err := acquireXRetainedQAProfileAtParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Release(true); err != nil {
		t.Fatal(err)
	}
	if _, err = acquireXRetainedProfileAtParent(parent, true); err == nil {
		t.Fatal("missing saved profile accepted")
	}
	parent = retainedSyntheticParent(t)
	lease, err = acquireXRetainedProfileAtParent(parent, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Release(false); err == nil {
		t.Fatal("unverified release reported clean")
	}
	for _, mode := range []bool{false, true} {
		if _, err = acquireXRetainedProfileAtParent(parent, mode); err == nil {
			t.Fatal("dirty state reused")
		}
	}
}

func TestXRetainedSharedRejectsChangedIdentity(t *testing.T) {
	for _, target := range []string{"root", "profile"} {
		t.Run(target, func(t *testing.T) {
			parent := retainedSyntheticParent(t)
			manifest, err := xRetainedReadManifest(filepath.Join(parent, xRetainedQAProfileManifestName))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, manifest.Root)
			if target == "profile" {
				path = filepath.Join(path, "shared")
			}
			if err = os.Rename(path, path+"-old"); err != nil {
				t.Fatal(err)
			}
			if err = os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []bool{false, true} {
				if _, err = acquireXRetainedProfileAtParent(parent, mode); err == nil {
					t.Fatal("replacement directory adopted")
				}
			}
		})
	}
}

func TestXRetainedOwnerCleanupBoundary(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified-before-launch", true: "unknown-cleanup"}[uncertain], func(t *testing.T) {
			parent := retainedSyntheticParent(t)
			lease, err := acquireXRetainedProfileAtParent(parent, true)
			if err != nil {
				t.Fatal(err)
			}
			owner := &xApplicationOwner{readerLease: lease, stopping: make(chan struct{}), closed: make(chan struct{})}
			if uncertain {
				owner.cleanupErr = context.DeadlineExceeded
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = owner.Close(ctx)
			if (err != nil) != uncertain {
				t.Fatal("cleanup verdict mismatch")
			}
			manifest, err := xRetainedReadManifest(filepath.Join(parent, xRetainedQAProfileManifestName))
			if err != nil {
				t.Fatal(err)
			}
			if uncertain {
				if manifest.State != "active" {
					t.Fatal("unknown cleanup published clean")
				}
				if _, err = acquireXRetainedProfileAtParent(parent, false); err == nil {
					t.Fatal("unknown cleanup released exclusion")
				}
				// Explicit synthetic-test teardown, preserving the persisted fence.
				lease.Release(false)
			} else {
				if manifest.State != "clean" {
					t.Fatal("verified pre-launch rollback did not clean")
				}
				next, err := acquireXRetainedProfileAtParent(parent, false)
				if err != nil {
					t.Fatal(err)
				}
				next.Close()
			}
		})
	}
}
