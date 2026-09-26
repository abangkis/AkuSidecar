package config

func windowsCaptureSplitEnabled(platform string, options Options) bool {
	return platform == "windows" && options.AppShell && options.WindowsCaptureSplit
}
