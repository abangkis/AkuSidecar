package captureruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type idleHoldProcess struct {
	*fakeProcess
	driver                string
	holds                 []bool
	failHold, failRelease bool
}

func (p *idleHoldProcess) Driver() string { return p.driver }
func (p *idleHoldProcess) SetIdleHold(ctx context.Context, held bool) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("unbounded idle acknowledgement")
	}
	p.holds = append(p.holds, held)
	if (held && p.failHold) || (!held && p.failRelease) {
		return errors.New("worker did not acknowledge")
	}
	return nil
}

func TestCollectionLeasesHoldIdleUntilLastRelease(t *testing.T) {
	p := &idleHoldProcess{fakeProcess: newProcess(100), driver: "headless"}
	m, _ := New(p)
	defer m.Terminate()
	first, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	first.Release()
	first.Release()
	if !reflect.DeepEqual(p.holds, []bool{true}) || m.Snapshot().ActiveLeases != 1 {
		t.Fatal("idle released while another batch was active", p.holds)
	}
	second.Release()
	if !reflect.DeepEqual(p.holds, []bool{true, false}) || m.Snapshot().ActiveLeases != 0 {
		t.Fatal("last lease did not release idle hold", p.holds)
	}
	third, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	third.Release()
	if !reflect.DeepEqual(p.holds, []bool{true, false, true, false}) {
		t.Fatal("next batch was not protected", p.holds)
	}
}

func TestIdleAcknowledgementFailureBlocksAdmission(t *testing.T) {
	for _, failAdmission := range []bool{true, false} {
		p := &idleHoldProcess{fakeProcess: newProcess(100), driver: "headless", failHold: failAdmission, failRelease: !failAdmission}
		m, _ := New(p)
		lease, err := m.Acquire()
		if failAdmission {
			if err == nil || lease != nil {
				t.Fatal("unacknowledged hold admitted work")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			lease.Release()
		}
		if m.Snapshot().State != Blocked || m.Snapshot().ActiveLeases != 0 {
			t.Fatal("uncertain idle state was not blocked", m.Snapshot())
		}
		if _, err := m.Acquire(); err == nil {
			t.Fatal("blocked owner admitted another batch")
		}
		m.Terminate()
	}
}

func TestBrowserLeasesNeverInvokeIdleCloseCapability(t *testing.T) {
	p := &idleHoldProcess{fakeProcess: newProcess(100), driver: "browser", failHold: true, failRelease: true}
	m, _ := New(p)
	defer m.Terminate()
	lease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if len(p.holds) != 0 {
		t.Fatal("user browser received an idle operation")
	}
}
