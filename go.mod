module github.com/unbox/unbox

go 1.27.3

// 注意：本文件含第 10 轮为升级而钉住的 47 个间接依赖版本（及后续轮次追加的钉）。
// 上游 go.mod 是剪裁过的，`go mod tidy` 会删掉这些钉、版本回落；不要直接跑 tidy，跑之前先备份 go.mod/go.sum。

require (
	github.com/PuerkitoBio/goquery v1.13.0
	github.com/dop251/goja v0.0.0-20261108153707-d8dfb4d8b999
	github.com/wailsapp/wails/v3 v3.1.0
	golang.org/x/sync v0.24.0
	golang.org/x/text v0.43.0
	modernc.org/sqlite v1.62.0
)

require (
	al.essio.dev/pkg/shellescape v1.6.1 // indirect
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.10 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	charm.land/lipgloss/v2 v2.0.6 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	cyphar.com/go-pathrs v0.2.6 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/Microsoft/go-winio v0.6.4 // indirect
	github.com/ProtonMail/go-crypto v1.5.2 // indirect
	github.com/ProtonMail/gopenpgp/v2 v2.11.1 // indirect
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/alecthomas/repr v0.5.4 // indirect
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/aymanbagabas/go-udiff v0.4.1 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.29.0 // indirect
	github.com/buger/jsonparser v1.6.1 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20261023060817-a45b7c86333f // indirect
	github.com/charmbracelet/x/ansi v0.11.9 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/exp/golden v0.1.0 // indirect
	github.com/charmbracelet/x/exp/slice v0.1.0 // indirect
	github.com/charmbracelet/x/exp/strings v0.1.0 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/cyphar/filepath-securejoin v0.7.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dlclark/regexp2/v2 v2.13.0 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260918173711-b481721df8a2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/elazarl/goproxy v1.9.3 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.2 // indirect
	github.com/go-git/go-git/v5 v5.19.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20261105003125-4bf881d7fb0b // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/assert v0.2.0 // indirect
	github.com/gookit/color v1.6.3 // indirect
	github.com/gopherjs/gopherjs v1.21.0 // indirect
	github.com/goreleaser/chglog v0.7.4 // indirect
	github.com/goreleaser/fileglob v1.4.1 // indirect
	github.com/goreleaser/nfpm/v2 v2.48.0 // indirect
	github.com/huandu/xstrings v1.6.3 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20261012173340-cbb8aaea4290 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/klauspost/cpuid/v2 v2.4.2 // indirect
	github.com/klauspost/pgzip v1.3.0 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/leaanthony/dmg v0.0.0-20260731075144-3d3581c3dfa3 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lmittmann/tint v1.2.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/mattn/go-zglob v0.0.8 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modelcontextprotocol/go-sdk v1.8.0 // indirect
	github.com/msvitok77/goembed v0.3.5 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/neelance/astrewrite v0.0.0-20160511093645-99348263ae86 // indirect
	github.com/neelance/sourcemap v0.0.0-20200213170602-2833bce08e4c // indirect
	github.com/onsi/gomega v1.44.0 // indirect
	github.com/pb33f/go-yaml v0.1.1 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.2 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/pjbgf/sha1cd v0.7.0 // indirect
	github.com/pterm/pterm v0.12.84 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/shopspring/decimal v1.6.0 // indirect
	github.com/shurcooL/go v0.0.0-20230706063926-5fe729b41b3a // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/skeema/knownhosts v1.3.3 // indirect
	github.com/smarty/assertions v1.16.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.1.0 // indirect
	github.com/stretchr/objx v0.8.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/ulikunitz/xz v0.5.17 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xo/terminfo v1.3.1 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	gitlab.com/digitalxero/go-conventional-commit v1.0.7 // indirect
	go.mozilla.org/pkcs7 v0.10.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp v0.0.0-20261106192419-47ce03d25ef0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261021172232-6cd571d43fbb // indirect
	golang.org/x/term v0.47.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.52.1-0.20261106145633-d407941a339a // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	modernc.org/cc/v4 v4.32.1 // indirect
	modernc.org/gc/v3 v3.2.1 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/opt v0.2.0 // indirect
	modernc.org/sortutil v1.2.1 // indirect
	modernc.org/strutil v1.2.1 // indirect
	modernc.org/token v1.1.0 // indirect
	mvdan.cc/sh/v3 v3.14.1 // indirect
	software.sslmate.com/src/go-pkcs12 v0.7.4 // indirect
)
