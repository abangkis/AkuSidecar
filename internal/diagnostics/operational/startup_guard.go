package operational

// StartupGuard represents CLI rejections whose fixed operator guidance is safe
// to include in a diagnostic. Other errors are projected by category only.
type StartupGuard string

const (
	DevBuildRequired        StartupGuard = "dev_build_required"
	DevProviderUnavailable  StartupGuard = "dev_provider_unavailable"
	DevSelectorRequired     StartupGuard = "dev_selector_required"
	FixtureGrantMissing     StartupGuard = "fixture_grant_missing"
	ConcurrentBuildRequired StartupGuard = "concurrent_build_required"
	QABuildRequired         StartupGuard = "qa_build_required"
	QAOptInRequired         StartupGuard = "qa_opt_in_required"
)

func (g StartupGuard) guidance() string {
	switch g {
	case DevBuildRequired:
		return "development session integration requires an auth_session_dev build"
	case DevProviderUnavailable:
		return "development session provider unavailable in this build"
	case DevSelectorRequired:
		return "development session requires --dev and supported synthetic-fixture provider"
	case FixtureGrantMissing:
		return "fixture startup grant missing"
	case ConcurrentBuildRequired:
		return "concurrent X capture requires the Windows auth_session_dev development build"
	case QABuildRequired:
		return "isolated X QA requires the Windows development build"
	case QAOptInRequired:
		return "isolated QA requires explicit supported development opt-in"
	default:
		return ""
	}
}

func (g StartupGuard) Error() string {
	if hint := g.guidance(); hint != "" {
		return hint
	}
	return "startup guard rejected"
}
