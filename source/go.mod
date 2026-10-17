module swiftuitutor

go 1.24.7

replace (
	golang.org/x/crypto => github.com/golang/crypto v0.43.0
	golang.org/x/sync => github.com/golang/sync v0.17.0
	golang.org/x/sys => github.com/golang/sys v0.37.0
	golang.org/x/text => github.com/golang/text v0.30.0
	lukechampine.com/adiantum => github.com/lukechampine/adiantum v1.1.1
)

require (
	github.com/ncruces/go-sqlite3 v0.30.5
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
)

require (
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)
