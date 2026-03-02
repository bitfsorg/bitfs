package dashboard

import "embed"

// FS contains the built dashboard files.
// To rebuild: cd dashboard && npm install && npm run build
//
//go:embed dist/*
var FS embed.FS
