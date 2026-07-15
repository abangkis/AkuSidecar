package httpapi

import "embed"

//go:embed web/*
var embeddedAssets embed.FS
