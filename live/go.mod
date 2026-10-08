// The acceptance suites, against a real Celeris stack.
module github.com/useceleris/sdk-go-server/live

go 1.27.0

require (
	github.com/useceleris/sdk-go-client v1.0.0
	github.com/useceleris/sdk-go-server v0.0.0-00010101000000-000000000000
)

require github.com/coder/websocket v1.8.15 // indirect

replace (
	// The client's working tree stands in until the version the server
	// requires is published.
	github.com/useceleris/sdk-go-client => ../../sdk-go-client
	github.com/useceleris/sdk-go-server => ../
)
