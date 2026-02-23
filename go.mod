module tideland.dev/go/boxcopy

go 1.24

require (
	github.com/emersion/go-imap/v2 v2.0.0-beta.5
	tideland.dev/go/asserts v0.3.0
	tideland.dev/go/toml v0.0.0
	tideland.dev/go/wait v0.0.0
	tideland.dev/go/worker v0.0.0
)

require (
	github.com/emersion/go-message v0.18.1 // indirect
	github.com/emersion/go-sasl v0.0.0-20231106173351-e73c9f7bad43 // indirect
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8 // indirect
	golang.org/x/time v0.5.0 // indirect
)

replace (
	tideland.dev/go/asserts => ../asserts
	tideland.dev/go/toml => ../toml
	tideland.dev/go/wait => ../wait
	tideland.dev/go/worker => ../worker
)
