module github.com/unbox/unbox

go 1.27.4

// 注意：本文件含第 10 轮为升级而钉住的 47 个间接依赖版本（及后续轮次追加的钉）。
// 上游 go.mod 是剪裁过的，`go mod tidy` 会删掉这些钉、版本回落；不要直接跑 tidy，跑之前先备份 go.mod/go.sum。

require (
	github.com/PuerkitoBio/goquery v1.13.0
	github.com/dop251/goja v0.0.0-20261110204830-f493a2b600d6
	github.com/wailsapp/wails/v3 v3.1.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	modernc.org/sqlite v1.62.0
)

require (
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dlclark/regexp2/v2 v2.13.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/google/pprof v0.0.0-20261111200114-fac431ef4d26 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/term v0.48.0 // indirect
	golang.org/x/tools v0.52.1-0.20261106145633-d407941a339a // indirect
	modernc.org/gc/v3 v3.2.1 // indirect
	modernc.org/libc v1.81.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
