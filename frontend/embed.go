package frontend

import "embed"

//go:embed index.html app.js style.css assets
var Assets embed.FS

//go:embed assets/app-icon.ico
var IconICO []byte

//go:embed assets/app-icon.png
var IconPNG []byte

//go:embed assets/tray-icon.png
var TrayPNG []byte
