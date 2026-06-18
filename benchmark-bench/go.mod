module github.com/itmo-vkr/dwss/benchmark-bench

go 1.22

require (
	github.com/itmo-vkr/dwss/internal/envcfg v0.0.0
	github.com/itmo-vkr/dwss/warmkit v0.0.0
)

require github.com/go-zookeeper/zk v1.0.4 // indirect

replace (
	github.com/itmo-vkr/dwss/internal/envcfg => ../internal/envcfg
	github.com/itmo-vkr/dwss/warmkit => ../warmkit
)
