//go:build auth_session_fixture

package collection

type SyntheticExclusiveHooks = ExclusiveSessionHooks

func (c *Coordinator) BindSyntheticExclusive(h SyntheticExclusiveHooks) error {
	return c.BindExclusiveSession(h)
}
func (c *Coordinator) SetSyntheticHeadlessCollector(generation uint64, backend CaptureBackend) {
	c.SetExclusiveHeadlessCollector(generation, backend)
}
func (c *Coordinator) SyntheticReaderPending() bool { return c.ExclusiveReaderPending() }
