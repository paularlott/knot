module github.com/paularlott/knot

go 1.27.2

//replace github.com/paularlott/gossip => ../gossip/

//replace github.com/paularlott/cli => ../cli

//replace github.com/paularlott/mcp => ../mcp

//replace github.com/paularlott/scriptling => ../scriptling

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/alicebob/miniredis/v2 v2.38.0
	github.com/creack/pty v1.1.24
	github.com/dgraph-io/badger/v4 v4.9.6
	github.com/fsnotify/fsnotify v1.10.1
	github.com/gliderlabs/ssh v0.3.8
	github.com/go-git/go-billy/v5 v5.9.2
	github.com/go-sql-driver/mysql v1.10.1
	github.com/go-webauthn/webauthn v0.18.2
	github.com/gogpu/systray v0.3.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/hashicorp/yamux v0.1.2
	github.com/jackc/pgx/v5 v5.12.0
	github.com/miekg/dns v1.1.73
	github.com/paularlott/cli v0.10.2
	github.com/paularlott/gossip v0.30.3
	github.com/paularlott/jsonrpc v0.3.0
	github.com/paularlott/lmchatkit v0.12.1
	github.com/paularlott/logger v0.4.0
	github.com/paularlott/mcp v0.33.1
	github.com/paularlott/scriptling v0.33.1
	github.com/pkg/sftp v1.13.12
	github.com/shamaton/msgpack/v3 v3.2.6
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/valkey-io/valkey-go v1.0.79
	github.com/willscott/go-nfs v0.0.5-0.20260926204150-6274d051a57f
	github.com/willscott/go-nfs-client v0.0.0-20251022144359-801f10d98886
	github.com/yeqown/go-qrcode/v2 v2.3.0
	github.com/yeqown/go-qrcode/writer/standard v1.4.0
	github.com/yuin/goldmark v1.8.7
	golang.org/x/crypto v0.58.0
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e
	golang.org/x/net v0.62.0
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.4.2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.3.1 // indirect
	github.com/go-webgpu/goffi v0.6.7 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20260802145828-341c2f0c90b5 // indirect
	github.com/paularlott/snapshotkv v0.6.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20260916203055-22a1a467d9f0 // indirect
	github.com/rasky/go-xdr v0.0.0-20170124162913-1a41d1a06c93 // indirect
	github.com/shoenig/go-m1cpu v0.2.4 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yeqown/reedsolomon v1.0.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/willscott/go-nfs => ./third_party/go-nfs
