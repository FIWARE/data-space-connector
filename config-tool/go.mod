module github.com/FIWARE/data-space-connector/config-tool

go 1.27.4

// The npm project lives in ui/, and some npm packages ship .go files.
ignore ./ui/node_modules

require (
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)
