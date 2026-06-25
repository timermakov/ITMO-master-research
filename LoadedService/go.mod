module github.com/itmo-vkr/dwss/LoadedService

go 1.22

require (
	github.com/edsrzf/mmap-go v1.1.0
	github.com/itmo-vkr/dwss/internal/envcfg v0.0.0
	github.com/itmo-vkr/dwss/internal/pprofserver v0.0.0
	github.com/itmo-vkr/dwss/warmkit v0.0.0
)

require (
	github.com/go-zookeeper/zk v1.0.4 // indirect
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e // indirect
)

replace (
	github.com/itmo-vkr/dwss/internal/envcfg => ../internal/envcfg
	github.com/itmo-vkr/dwss/internal/pprofserver => ../internal/pprofserver
	github.com/itmo-vkr/dwss/warmkit => ../warmkit
)
