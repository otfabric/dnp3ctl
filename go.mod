module github.com/otfabric/dnp3ctl

go 1.25.0

require (
	github.com/otfabric/go-dnp3 v0.0.0
	github.com/otfabric/go-serial v0.1.6
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/inconshreveable/mousetrap v1.1.0 // indirect

// go-dnp3 is not tagged yet: build against the sibling checkout. Drop this line and pin a
// version once it is released (see RELEASE.md).
replace github.com/otfabric/go-dnp3 => ../go-dnp3
